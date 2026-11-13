//! Read-only MyFinder observation. The existing native reconciliation worker
//! is the only writer; preferences and derived geometry are never persisted.
use serde::Serialize;
use std::sync::{Mutex, OnceLock};
use std::sync::atomic::{AtomicU8, Ordering};
use std::time::Duration;
use tauri::{AppHandle, Emitter, Manager};
use windows::Win32::Foundation::{HWND, LPARAM, POINT, RECT};
use windows::Win32::Graphics::Dwm::{DwmGetWindowAttribute, DWMWA_CLOAKED};
use windows::Win32::Graphics::Gdi::{
    BitBlt, CreateCompatibleBitmap, CreateCompatibleDC, DeleteDC, DeleteObject, GetDC, GetDIBits,
    GetPixel, ReleaseDC, SelectObject, BITMAPINFO, BITMAPINFOHEADER, DIB_RGB_COLORS, SRCCOPY,
};
use windows::Win32::UI::WindowsAndMessaging::{
    EnumWindows, GetClassNameW, GetLayeredWindowAttributes, GetWindowLongW, GetWindowRect,
    GetWindowThreadProcessId, IsWindowVisible, GWL_EXSTYLE, LWA_ALPHA, WS_EX_LAYERED,
};

#[derive(Clone, Debug, PartialEq, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct Snapshot {
    pub revision: u64,
    pub available: bool,
    pub hidden: bool,
    pub height: i32,
    pub color: Option<[u8; 3]>,
    /// Right edge of MyFinder's own menu, in physical pixels from the
    /// monitor's left edge. 0 when it is not known.
    pub menu_right: i32,
}

static SNAPSHOT: Mutex<Snapshot> = Mutex::new(Snapshot {
    revision: 0,
    available: false,
    hidden: false,
    height: 0,
    color: None,
    menu_right: 0,
});
static OWNER: OnceLock<std::thread::Thread> = OnceLock::new();
#[derive(Clone, Copy, PartialEq)]
struct Identity {
    handle: usize,
    pid: u32,
    monitor: (i32, i32, i32, i32),
}
static IDENTITY: Mutex<Option<Identity>> = Mutex::new(None);

pub fn register_owner() {
    let _ = OWNER.set(std::thread::current());
}

pub fn request_reconcile() {
    if let Some(worker) = OWNER.get() {
        worker.unpark();
    }
}

/// MyFinder repaints its menu a moment after the foreground app changes, so
/// one immediate scan can still read the old menu. Ask for a few quick ones.
static FOLLOW_UPS: AtomicU8 = AtomicU8::new(0);

pub fn menu_may_change() {
    FOLLOW_UPS.store(4, Ordering::Relaxed);
    request_reconcile();
}

/// How long the reconciliation worker sleeps before its next pass.
pub fn next_wait() -> Duration {
    let pending = FOLLOW_UPS
        .try_update(Ordering::Relaxed, Ordering::Relaxed, |n| n.checked_sub(1))
        .is_ok();
    if pending {
        Duration::from_millis(150)
    } else {
        Duration::from_secs(2)
    }
}

pub fn hidden() -> bool {
    SNAPSHOT
        .lock()
        .map(|s| s.available && s.hidden)
        .unwrap_or(false)
}

#[tauri::command]
pub fn get_myfinder_state() -> Snapshot {
    SNAPSHOT.lock().map(|s| s.clone()).unwrap_or_default()
}

pub fn event(hwnd: HWND) {
    let known = IDENTITY
        .lock()
        .ok()
        .and_then(|v| *v)
        .is_some_and(|id| id.handle == hwnd.0 as usize);
    let mut class = [0u16; 64];
    let len = unsafe { GetClassNameW(hwnd, &mut class) };
    if known || String::from_utf16_lossy(&class[..len.max(0) as usize]) == "MyFinderApp" {
        request_reconcile();
    }
}

/// A bar may slide upward while hiding. Its identity is kept across that
/// transition, but a different display/handle must establish its own geometry.
fn fits_bar(bar: RECT, monitor: RECT) -> bool {
    let height = bar.bottom - bar.top;
    let centre = monitor.left + (monitor.right - monitor.left) / 2;
    height > 0
        && height <= 160
        && bar.left <= centre
        && bar.right > centre
        && bar.top <= monitor.top + 2
        && bar.bottom >= monitor.top - height
}

pub fn reconcile(app: &AppHandle, monitor: RECT) {
    struct Candidate {
        hwnd: HWND,
        pid: u32,
        rect: RECT,
    }
    unsafe extern "system" fn find(hwnd: HWND, data: LPARAM) -> windows::core::BOOL {
        let candidates = &mut *(data.0 as *mut Vec<Candidate>);
        let mut class = [0u16; 64];
        let len = GetClassNameW(hwnd, &mut class);
        if String::from_utf16_lossy(&class[..len.max(0) as usize]) != "MyFinderApp" {
            return true.into();
        }
        let mut pid = 0;
        GetWindowThreadProcessId(hwnd, Some(&mut pid));
        // Validate process as well as class: HWNDs are routinely reused.
        use windows::Win32::System::Threading::{
            OpenProcess, QueryFullProcessImageNameW, PROCESS_NAME_WIN32,
            PROCESS_QUERY_LIMITED_INFORMATION,
        };
        if let Ok(process) = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid) {
            let mut path = [0u16; 1024];
            let mut len = path.len() as u32;
            let valid = QueryFullProcessImageNameW(
                process,
                PROCESS_NAME_WIN32,
                windows::core::PWSTR(path.as_mut_ptr()),
                &mut len,
            )
            .is_ok()
                && String::from_utf16_lossy(&path[..len as usize])
                    .rsplit('\\')
                    .next()
                    .is_some_and(|name| {
                        name.eq_ignore_ascii_case("Dock_64.exe")
                            || name.eq_ignore_ascii_case("Dock.exe")
                    });
            let _ = windows::Win32::Foundation::CloseHandle(process);
            let mut rect = RECT::default();
            if valid && GetWindowRect(hwnd, &mut rect).is_ok() {
                candidates.push(Candidate { hwnd, pid, rect });
            }
        }
        true.into()
    }
    let mut candidates = Vec::<Candidate>::new();
    let success =
        unsafe { EnumWindows(Some(find), LPARAM(&mut candidates as *mut _ as isize)).is_ok() };
    let candidate = candidates.into_iter().find(|c| fits_bar(c.rect, monitor));
    let old_identity = IDENTITY.lock().ok().and_then(|v| *v);
    let mut next = Snapshot::default();
    let mut identity = None;
    if success {
        if let Some(c) = candidate {
            identity = Some(Identity {
                handle: c.hwnd.0 as usize,
                pid: c.pid,
                monitor: (monitor.left, monitor.top, monitor.right, monitor.bottom),
            });
            let prior = get_myfinder_state();
            let same = identity == old_identity;
            let mut cloaked: u32 = 0;
            let cloak_known = unsafe {
                DwmGetWindowAttribute(
                    c.hwnd,
                    DWMWA_CLOAKED,
                    &mut cloaked as *mut _ as *mut _,
                    std::mem::size_of::<u32>() as u32,
                )
                .is_ok()
            };
            let mut alpha = 255u8;
            unsafe {
                if GetWindowLongW(c.hwnd, GWL_EXSTYLE) as u32 & WS_EX_LAYERED.0 != 0 {
                    let mut flags = Default::default();
                    if GetLayeredWindowAttributes(c.hwnd, None, Some(&mut alpha), Some(&mut flags))
                        .is_err()
                        || flags.0 & LWA_ALPHA.0 == 0
                    {
                        alpha = 255;
                    }
                }
            }
            let hidden = !unsafe { IsWindowVisible(c.hwnd).as_bool() }
                || (cloak_known && cloaked != 0)
                || alpha == 0
                || c.rect.bottom <= monitor.top + 2;
            next = Snapshot {
                available: true,
                hidden,
                height: if hidden && same && prior.height > 0 {
                    prior.height
                } else {
                    c.rect.bottom - c.rect.top
                },
                color: if hidden && same {
                    prior.color
                } else {
                    sample_material(c.hwnd, c.rect).or(if same { prior.color } else { None })
                },
                menu_right: if hidden {
                    if same {
                        prior.menu_right
                    } else {
                        0
                    }
                } else {
                    let limit = bloom_bar_left(app)
                        .unwrap_or(monitor.left + (monitor.right - monitor.left) / 2);
                    scan_menu_right(c.hwnd, c.rect, limit)
                        .map(|x| (x - monitor.left).max(0))
                        .unwrap_or(if same { prior.menu_right } else { 0 })
                },
                ..Default::default()
            };
        }
    }
    if let Ok(mut slot) = IDENTITY.lock() {
        *slot = identity;
    }
    if let Ok(mut current) = SNAPSHOT.lock() {
        next.revision = current.revision;
        if *current != next {
            next.revision += 1;
            *current = next.clone();
            let _ = app.emit("myfinder-changed", next);
        }
    }
}

/// Sample only a few material pixels away from Bloom's centre footprint.
/// No image or foreign settings are captured, cached on disk, or transmitted.
fn sample_material(hwnd: HWND, rect: RECT) -> Option<[u8; 3]> {
    unsafe {
        // MyFinder composites its material outside the window's ordinary
        // paint DC (which can return white). Read the visible material only
        // where the hit window still belongs to MyFinder, never an app above it.
        let dc = GetDC(None);
        if dc.0.is_null() {
            return None;
        }
        let width = rect.right - rect.left;
        let y = rect.bottom - 2;
        let mut values = Vec::new();
        for part in [1, 2, 6, 7] {
            use windows::Win32::UI::WindowsAndMessaging::{GetAncestor, WindowFromPoint, GA_ROOT};
            let point = POINT {
                x: rect.left + width * part / 8,
                y,
            };
            if GetAncestor(WindowFromPoint(point), GA_ROOT) != hwnd {
                continue;
            }
            let pixel = GetPixel(dc, point.x, point.y).0;
            if pixel != u32::MAX {
                values.push([
                    (pixel & 255) as u8,
                    ((pixel >> 8) & 255) as u8,
                    ((pixel >> 16) & 255) as u8,
                ]);
            }
        }
        ReleaseDC(None, dc);
        if values.len() < 3 {
            return None;
        }
        Some(std::array::from_fn(|channel| {
            values.sort_by_key(|c| c[channel]);
            values[values.len() / 2][channel]
        }))
    }
}

/// Screen x of the left edge of what Bloom paints in the bar row. The menu
/// scan stops there, so Bloom never reads its own pixels as menu text.
fn bloom_bar_left(app: &AppHandle) -> Option<i32> {
    let left = crate::window_region::main_bar_left()?;
    let hwnd = crate::utils::frame_hwnd(app.get_webview_window("main")?.hwnd().ok()?);
    let mut rect = RECT::default();
    unsafe { GetWindowRect(hwnd, &mut rect).ok()? };
    Some(rect.left + left)
}

/// Columns whose pixels differ this much from top to bottom hold a glyph or an
/// icon. MyFinder's blurred material changes far less over the same rows.
const MENU_CONTRAST: u8 = 48;

/// Index one past the last column that holds menu content, or 0 for none.
/// `pixels` is `rows` lines of `width` four-byte pixels. A lone column is
/// ignored: text and icons are always at least two columns wide.
fn menu_edge(pixels: &[u8], width: usize, rows: usize) -> usize {
    let busy = |x: usize| {
        (0..3).any(|channel| {
            let column = (0..rows).map(|y| pixels[(y * width + x) * 4 + channel]);
            let (low, high) = column.fold((255u8, 0u8), |(l, h), v| (l.min(v), h.max(v)));
            high - low >= MENU_CONTRAST
        })
    };
    let mut x = width;
    while x >= 2 {
        if busy(x - 1) && busy(x - 2) {
            return x;
        }
        x -= 1;
    }
    0
}

/// Screen x of the right edge of MyFinder's menu, read from the bar's own
/// pixels between its left edge and `limit`. MyFinder exposes no child window
/// or accessibility element for the menu. The copied rows stay in memory for
/// this one measurement and are never stored or sent anywhere.
fn scan_menu_right(hwnd: HWND, rect: RECT, limit: i32) -> Option<i32> {
    use windows::Win32::UI::WindowsAndMessaging::{GetAncestor, WindowFromPoint, GA_ROOT};
    let height = rect.bottom - rect.top;
    let width = limit.min(rect.right) - rect.left;
    // The middle half of the bar: clear of its border and of any drop shadow.
    let rows = (height / 2).max(1);
    let top = rect.top + height / 4;
    if width < 2 {
        return Some(rect.left);
    }
    unsafe {
        let probe = POINT {
            x: rect.left + 4,
            y: top + rows / 2,
        };
        if GetAncestor(WindowFromPoint(probe), GA_ROOT) != hwnd {
            return None;
        }
        let screen = GetDC(None);
        if screen.0.is_null() {
            return None;
        }
        let memory = CreateCompatibleDC(Some(screen));
        let bitmap = CreateCompatibleBitmap(screen, width, rows);
        let previous = SelectObject(memory, bitmap.into());
        let copied =
            BitBlt(memory, 0, 0, width, rows, Some(screen), rect.left, top, SRCCOPY).is_ok();
        SelectObject(memory, previous);
        let mut info = BITMAPINFO {
            bmiHeader: BITMAPINFOHEADER {
                biSize: std::mem::size_of::<BITMAPINFOHEADER>() as u32,
                biWidth: width,
                biHeight: -rows,
                biPlanes: 1,
                biBitCount: 32,
                ..Default::default()
            },
            ..Default::default()
        };
        let mut pixels = vec![0u8; (width * rows * 4) as usize];
        let read = copied
            && GetDIBits(
                memory,
                bitmap,
                0,
                rows as u32,
                Some(pixels.as_mut_ptr() as *mut _),
                &mut info,
                DIB_RGB_COLORS,
            ) == rows;
        let _ = DeleteObject(bitmap.into());
        let _ = DeleteDC(memory);
        ReleaseDC(None, screen);
        read.then(|| rect.left + menu_edge(&pixels, width as usize, rows as usize) as i32)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn menu_edge_is_past_the_last_glyph_and_ignores_smooth_material() {
        let (width, rows) = (40usize, 6usize);
        // A smooth left-to-right gradient: no column varies top to bottom.
        let mut pixels = vec![0u8; width * rows * 4];
        for y in 0..rows {
            for x in 0..width {
                for channel in 0..3 {
                    pixels[(y * width + x) * 4 + channel] = 120 + x as u8;
                }
            }
        }
        assert_eq!(menu_edge(&pixels, width, rows), 0);
        // A dark glyph three columns wide, and one stray column further right.
        for x in [10usize, 11, 12, 30] {
            for channel in 0..3 {
                pixels[(2 * width + x) * 4 + channel] = 0;
            }
        }
        assert_eq!(menu_edge(&pixels, width, rows), 13);
    }
    #[test]
    fn accepts_visible_and_slid_bar_on_the_target_monitor_only() {
        let monitor = RECT {
            left: -1920,
            top: 0,
            right: 0,
            bottom: 1080,
        };
        for top in [0, -29, -30] {
            assert!(fits_bar(
                RECT {
                    left: -1920,
                    top,
                    right: 0,
                    bottom: top + 30
                },
                monitor
            ));
        }
        assert!(!fits_bar(
            RECT {
                left: 0,
                top: 0,
                right: 1920,
                bottom: 30
            },
            monitor
        ));
        assert!(!fits_bar(
            RECT {
                left: -1920,
                top: 24,
                right: 0,
                bottom: 54
            },
            monitor
        ));
        assert!(!fits_bar(
            RECT {
                left: -1920,
                top: 0,
                right: 0,
                bottom: 0
            },
            monitor
        ));
    }
}
