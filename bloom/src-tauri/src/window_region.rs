//! Clips a Bloom window to the parts of the screen its UI actually draws on.
//!
//! Bloom's windows are large transparent rectangles (the notch window spans
//! the whole top band of the screen). Wherever such a window catches input,
//! it swallows clicks meant for the apps underneath, even where it draws
//! nothing. A window region (`SetWindowRgn`) limits both drawing and hit
//! testing to the given rectangles, so outside them the window doesn't exist
//! for input at all. Each webview reports the rectangles it needs, in its own
//! CSS pixels, whenever they change.

use tauri::Window;
use windows::Win32::Foundation::RECT;
use windows::Win32::Graphics::Gdi::{
    CombineRgn, CreateRectRgn, DeleteObject, SetWindowRgn, RGN_OR,
};

use crate::types::IntRect;
use std::sync::Mutex;

static MAIN_REGIONS: Mutex<Vec<RECT>> = Mutex::new(Vec::new());

pub fn main_contains(x: i32, y: i32) -> bool {
    MAIN_REGIONS
        .lock()
        .map(|rects| {
            rects
                .iter()
                .any(|r| x >= r.left && x < r.right && y >= r.top && y < r.bottom)
                || bridges_gap(&rects, x, y)
        })
        .unwrap_or(false)
}

/// Left edge, in physical window pixels, of what the notch window paints in
/// the top row. None while it paints nothing there.
pub fn main_bar_left() -> Option<i32> {
    bar_left(&MAIN_REGIONS.lock().ok()?)
}

fn bar_left(rects: &[RECT]) -> Option<i32> {
    rects.iter().filter(|r| r.top <= 2).map(|r| r.left).min()
}

/// Widest vertical gap, in physical pixels, that still counts as one surface.
const MAX_BRIDGED_GAP: i32 = 32;

/// MyFinder's menu floats a few pixels below the bar. The window region must
/// not cover that gap (an unpainted region pixel shows as a pale haze), yet a
/// pointer crossing it is still on its way from the bar to the menu: reporting
/// it as outside would flip the window click-through and close the menu under
/// the cursor. Clicks in the gap still reach the app below, since the OS
/// hit-tests against the region itself.
fn bridges_gap(rects: &[RECT], x: i32, y: i32) -> bool {
    rects.iter().any(|upper| {
        rects.iter().any(|lower| {
            let gap = lower.top - upper.bottom;
            gap > 0
                && gap <= MAX_BRIDGED_GAP
                && y >= upper.bottom
                && y < lower.top
                && x >= upper.left.max(lower.left)
                && x < upper.right.min(lower.right)
        })
    })
}

/// The rectangles in physical window pixels: scaled by the window's DPI scale,
/// rounded outwards so nothing drawn at a fractional edge is clipped, and with
/// empty ones dropped.
fn physical_rects(rects: &[IntRect], scale: f64) -> Vec<RECT> {
    rects
        .iter()
        .filter(|r| r.width > 0 && r.height > 0)
        .map(|r| RECT {
            left: (r.x as f64 * scale).floor() as i32,
            top: (r.y as f64 * scale).floor() as i32,
            right: ((r.x + r.width) as f64 * scale).ceil() as i32,
            bottom: ((r.y + r.height) as f64 * scale).ceil() as i32,
        })
        .collect()
}

/// Sets the calling window's region to the union of `rects` (CSS pixels of
/// the window). An empty list leaves the window with an empty region: it draws
/// nothing and takes no input until the next call.
#[tauri::command]
pub fn set_window_region(window: Window, rects: Vec<IntRect>) {
    let Ok(hwnd) = window.hwnd() else {
        return;
    };
    // hwnd() may resolve to a child host; the region must clip the visible
    // root frame, or the frame keeps hit-testing stale (or everywhere).
    let hwnd = crate::utils::frame_hwnd(hwnd);
    let scale = window.scale_factor().unwrap_or(1.0);
    let physical = physical_rects(&rects, scale);
    // A region on a window that isn't layered turns its transparent pixels
    // opaque (see set_click_through). Make sure it's layered before the region
    // lands, keeping whatever click-through state it has.
    let click_through = unsafe {
        use windows::Win32::UI::WindowsAndMessaging::{
            GetWindowLongPtrW, GWL_EXSTYLE, WS_EX_TRANSPARENT,
        };
        GetWindowLongPtrW(hwnd, GWL_EXSTYLE) as u32 & WS_EX_TRANSPARENT.0 != 0
    };
    crate::utils::set_click_through(hwnd, click_through);
    unsafe {
        let region = CreateRectRgn(0, 0, 0, 0);
        for rect in &physical {
            let part = CreateRectRgn(rect.left, rect.top, rect.right, rect.bottom);
            CombineRgn(Some(region), Some(region), Some(part), RGN_OR);
            let _ = DeleteObject(part.into());
        }
        // The system owns the region from here on, so it isn't deleted on
        // success. Redraw so the newly uncovered parts paint.
        if SetWindowRgn(hwnd, Some(region), true) == 0 {
            let _ = DeleteObject(region.into());
        } else if window.label() == "main" {
            if let Ok(mut slot) = MAIN_REGIONS.lock() {
                // The menu scan stops at this edge, so a move can uncover or
                // hide menu text: measure again.
                let moved = bar_left(&slot) != bar_left(&physical);
                *slot = physical;
                drop(slot);
                if moved {
                    crate::myfinder::request_reconcile();
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rect(x: i32, y: i32, width: i32, height: i32) -> IntRect {
        IntRect {
            x,
            y,
            width,
            height,
        }
    }

    #[test]
    fn scales_and_rounds_outwards() {
        let scaled = physical_rects(&[rect(10, 0, 101, 45)], 1.25);
        assert_eq!(
            scaled,
            vec![RECT {
                left: 12,
                top: 0,
                right: 139,
                bottom: 57
            }]
        );
    }

    fn physical(left: i32, top: i32, right: i32, bottom: i32) -> RECT {
        RECT {
            left,
            top,
            right,
            bottom,
        }
    }

    #[test]
    fn bridges_the_gap_between_bar_and_menu() {
        let rects = [physical(100, 0, 500, 30), physical(50, 40, 550, 400)];
        assert!(bridges_gap(&rects, 300, 35));
        // Outside the shared horizontal span, or beyond either surface.
        assert!(!bridges_gap(&rects, 60, 35));
        assert!(!bridges_gap(&rects, 300, 20));
        assert!(!bridges_gap(&rects, 300, 40));
    }

    #[test]
    fn does_not_bridge_distant_or_overlapping_surfaces() {
        assert!(!bridges_gap(
            &[physical(0, 0, 100, 30), physical(0, 80, 100, 200)],
            50,
            50
        ));
        assert!(!bridges_gap(
            &[physical(0, 0, 100, 30), physical(0, 20, 100, 200)],
            50,
            25
        ));
    }

    #[test]
    fn drops_empty_rectangles() {
        let scaled = physical_rects(&[rect(0, -100, 140, 0), rect(5, 5, 0, 10)], 2.0);
        assert!(scaled.is_empty());
    }
}
