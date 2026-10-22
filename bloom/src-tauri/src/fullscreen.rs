//! Fullscreen suppression for the main notch window.
//!
//! The dock only cares about taskbar-like overlap, so `services.rs` asks a
//! different question for the notch: does the foreground window own the
//! primary monitor? This module probes that window once per visibility tick
//! and `notch_hidden_for` turns the probe into a hide/show decision.

use windows::Win32::Foundation::HWND;

/// Shell-owned classes that can cover the primary monitor without being a
/// fullscreen app: the desktop, the taskbars, and XAML shell surfaces.
const SHELL_CLASS_NAMES: &[&str] = &[
    "Progman",
    "WorkerW",
    "Shell_TrayWnd",
    "Shell_SecondaryTrayWnd",
    "XamlExplorerHostIslandWindow",
    "Windows.UI.Core.CoreWindow",
    "MultitaskingViewFrame",
    "ForegroundStaging",
];

/// A snapshot of the foreground window taken on a single visibility-loop
/// tick. Every field is read from the same hwnd, so the decision never mixes
/// attributes from two different windows.
pub struct ForegroundProbe {
    /// The window's frame bounds cover the primary monitor's screen rect.
    pub covers_primary: bool,
    /// WS_EX_TOOLWINDOW: small floating helpers never suppress the notch.
    pub tool_window: bool,
    /// WS_EX_NOACTIVATE: overlay/flyout windows never suppress the notch.
    pub no_activate: bool,
    /// WS_CAPTION: a titled window is at most a maximized window.
    pub has_caption: bool,
    /// The window's class name, read from this hwnd on this tick.
    pub class_name: String,
    /// SHQueryUserNotificationState: 3 = exclusive fullscreen, 4 = presentation.
    pub notification_state: u32,
}

impl ForegroundProbe {
    /// Reads every field from `hwnd` right now. `None` when the loop holds no
    /// foreground window, which the caller treats as "do not hide".
    pub fn from_hwnd(hwnd: HWND) -> Option<ForegroundProbe> {
        use windows::Win32::Foundation::{POINT, RECT};
        use windows::Win32::Graphics::Dwm::{DwmGetWindowAttribute, DWMWA_EXTENDED_FRAME_BOUNDS};
        use windows::Win32::Graphics::Gdi::{
            GetMonitorInfoA, MonitorFromPoint, MONITORINFO, MONITOR_DEFAULTTOPRIMARY,
        };
        use windows::Win32::UI::Shell::SHQueryUserNotificationState;
        use windows::Win32::UI::WindowsAndMessaging::{
            GetClassNameA, GetWindowLongW, GetWindowRect, GWL_EXSTYLE, GWL_STYLE, WS_CAPTION,
            WS_EX_NOACTIVATE, WS_EX_TOOLWINDOW,
        };

        if hwnd.is_invalid() {
            return None;
        }

        unsafe {
            let mut class_buf = [0u8; 256];
            let class_len = GetClassNameA(hwnd, &mut class_buf);
            let class_name =
                String::from_utf8_lossy(&class_buf[..class_len as usize]).into_owned();

            let style = GetWindowLongW(hwnd, GWL_STYLE) as u32;
            let ex_style = GetWindowLongW(hwnd, GWL_EXSTYLE) as u32;

            // Extended frame bounds match what the user sees; fall back to the
            // plain window rect when DWM cannot report them.
            let mut rect = RECT::default();
            if DwmGetWindowAttribute(
                hwnd,
                DWMWA_EXTENDED_FRAME_BOUNDS,
                &mut rect as *mut _ as *mut _,
                std::mem::size_of::<RECT>() as u32,
            )
            .is_err()
                && GetWindowRect(hwnd, &mut rect).is_err()
            {
                return Some(ForegroundProbe {
                    covers_primary: false,
                    tool_window: (ex_style & WS_EX_TOOLWINDOW.0) != 0,
                    no_activate: (ex_style & WS_EX_NOACTIVATE.0) != 0,
                    has_caption: (style & WS_CAPTION.0) != 0,
                    class_name,
                    notification_state: SHQueryUserNotificationState()
                        .map(|s| s.0 as u32)
                        .unwrap_or(0),
                });
            }

            // (0,0) is always the primary monitor's origin, so this resolves
            // the primary even when the window sits nearest another monitor.
            let primary = MonitorFromPoint(POINT { x: 0, y: 0 }, MONITOR_DEFAULTTOPRIMARY);
            let mut mi = MONITORINFO {
                cbSize: std::mem::size_of::<MONITORINFO>() as u32,
                ..Default::default()
            };
            let covers_primary = GetMonitorInfoA(primary, &mut mi).as_bool()
                && rect.left <= mi.rcMonitor.left
                && rect.top <= mi.rcMonitor.top
                && rect.right >= mi.rcMonitor.right
                && rect.bottom >= mi.rcMonitor.bottom;

            Some(ForegroundProbe {
                covers_primary,
                tool_window: (ex_style & WS_EX_TOOLWINDOW.0) != 0,
                no_activate: (ex_style & WS_EX_NOACTIVATE.0) != 0,
                has_caption: (style & WS_CAPTION.0) != 0,
                class_name,
                notification_state: SHQueryUserNotificationState()
                    .map(|s| s.0 as u32)
                    .unwrap_or(0),
            })
        }
    }
}

/// Whether the main notch should hide for this foreground window.
///
/// True when the shell reports exclusive fullscreen (3) or presentation
/// mode (4) and the window covers the primary monitor. Otherwise true only
/// for a borderless window that owns the primary: no tool window, no
/// no-activate flag, no caption, and not a known shell class.
pub fn notch_hidden_for(probe: &ForegroundProbe) -> bool {
    if !probe.covers_primary {
        return false;
    }
    if probe.notification_state == 3 || probe.notification_state == 4 {
        return true;
    }
    if probe.tool_window || probe.no_activate || probe.has_caption {
        return false;
    }
    !SHELL_CLASS_NAMES.contains(&probe.class_name.as_str())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn probe(
        covers_primary: bool,
        tool_window: bool,
        no_activate: bool,
        has_caption: bool,
        class_name: &str,
        notification_state: u32,
    ) -> ForegroundProbe {
        ForegroundProbe {
            covers_primary,
            tool_window,
            no_activate,
            has_caption,
            class_name: class_name.to_string(),
            notification_state,
        }
    }

    #[test]
    fn exclusive_fullscreen_hides() {
        // State 3 covers the primary even when the window keeps a caption.
        assert!(notch_hidden_for(&probe(
            true, false, false, true, "MyGame", 3
        )));
    }

    #[test]
    fn presentation_hides() {
        assert!(notch_hidden_for(&probe(
            true, false, false, false, "POWERPNT", 4
        )));
    }

    #[test]
    fn toolwindow_does_not_hide() {
        assert!(!notch_hidden_for(&probe(
            true, true, false, false, "MyTool", 2
        )));
    }

    #[test]
    fn titled_maximized_does_not_hide() {
        assert!(!notch_hidden_for(&probe(
            true, false, false, true, "Chrome_WidgetWin_1", 2
        )));
    }

    #[test]
    fn borderless_covering_primary_hides() {
        // A borderless, activating, non-tool window that covers the primary
        // is a fullscreen surface even when the shell reports no fullscreen
        // state.
        assert!(notch_hidden_for(&probe(
            true, false, false, false, "SDL_app", 5
        )));
    }
}
