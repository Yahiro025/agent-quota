//! Full hide for the main notch window over exclusive fullscreen.
//!
//! Every other fullscreen app only retracts the notch (the visibility loop's
//! `visibility-change` event), so a top-centre dwell can still reveal it. An
//! exclusive D3D app is different: a topmost window over it can knock it out
//! of exclusive mode, so the notch window is hidden outright there.

use windows::Win32::Foundation::HWND;

/// A snapshot of the foreground window taken on a single visibility-loop tick.
pub struct ForegroundProbe {
    /// The window's frame bounds cover the primary monitor's screen rect.
    pub covers_primary: bool,
    /// SHQueryUserNotificationState: 3 = exclusive D3D fullscreen.
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
        use windows::Win32::UI::WindowsAndMessaging::GetWindowRect;

        if hwnd.is_invalid() {
            return None;
        }

        unsafe {
            let notification_state = SHQueryUserNotificationState()
                .map(|s| s.0 as u32)
                .unwrap_or(0);

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
                    notification_state,
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
                notification_state,
            })
        }
    }
}

/// Whether the main notch window should be hidden outright: only for an
/// exclusive D3D app (state 3) that owns the primary monitor.
pub fn notch_hidden_for(probe: &ForegroundProbe) -> bool {
    probe.covers_primary && probe.notification_state == 3
}

#[cfg(test)]
mod tests {
    use super::*;

    fn probe(covers_primary: bool, notification_state: u32) -> ForegroundProbe {
        ForegroundProbe {
            covers_primary,
            notification_state,
        }
    }

    #[test]
    fn exclusive_fullscreen_hides() {
        assert!(notch_hidden_for(&probe(true, 3)));
    }

    #[test]
    fn exclusive_fullscreen_on_other_monitor_does_not_hide() {
        assert!(!notch_hidden_for(&probe(false, 3)));
    }

    #[test]
    fn presentation_and_borderless_only_retract() {
        // Presentation (4), busy (2) and plain borderless (5) stay revealable.
        for state in [2, 4, 5] {
            assert!(!notch_hidden_for(&probe(true, state)));
        }
    }
}
