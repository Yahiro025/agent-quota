# Top-Edge Seam Over Top Bars — Design Spec

Date: 2026-10-21
Status: Approved for implementation
Wayfinder map: `.scratch/top-edge-notch/map.md`

## 1. Problem & Goal

On Windows setups with a top appbar such as MyDockFinder (`Dock_64.exe`, class `MyFinderApp`, occupying `(0,0)–(1920,30)` physical px at 125% DPI) or a top-docked taskbar, `SPI_GETWORKAREA` (`win.WorkArea()`) returns `Min.Y > 0` (`30` physical px / `24` logical px). Because `agent-quota` previously clamped Seam placement and top-edge attachment (`v.Floating = a.y > work.Min.Y`) to `win.WorkArea()`, the Seam notch could never reach the physical top edge of the monitor (`y = 0`) and instead hung below MyDockFinder's bar. Additionally, `agent-quota` had no fullscreen suppression for Seam, leaving the topmost window visible over fullscreen applications.

**Goal**: Make Seam behave like a real hardware-bezel notch at the top center of the primary monitor (`y = 0`), overlaying MyDockFinder or any top appbar as one continuous shape (both collapsed rail and open overview), maintaining topmost z-order above the bar, passing clicks outside the notch contour through to the bar, and hiding cleanly while a fullscreen app occupies the primary monitor.

## 2. Geometry & Placement Rules

1. **Primary Monitor Bounds (`win.MonitorBounds() image.Rectangle`)**:
   - Query `MonitorFromPoint(0, MONITOR_DEFAULTTOPRIMARY)` (`1`) and `GetMonitorInfoW` to return `rcMonitor` in physical pixels (falling back to `image.Rect(0, 0, ScreenSize())`). On the primary display, `MonitorBounds().Min.Y == 0`.
2. **Seam Placement Rectangle (`seamWorkArea(work, monitor image.Rectangle) image.Rectangle` / `win.SeamWorkArea() image.Rectangle`)**:
   - Given `work = win.WorkArea()` and `monitor = win.MonitorBounds()`, `seamWorkArea(work, monitor)` returns:
     ```go
     image.Rect(work.Min.X, monitor.Min.Y, work.Max.X, work.Max.Y)
     ```
     (falling back to `work` if `monitor.Empty()` or `monitor.Min.Y > work.Min.Y`).
   - This lifts the top ceiling from `work.Min.Y` (`30` physical px under MyDockFinder) to `monitor.Min.Y` (`0`), while preserving `work.Min.X`, `work.Max.X`, and `work.Max.Y` so side and bottom taskbars remain clear.
   - Taskbar Focus (`taskbar.go` `focusPopupPosition` and `v.FocusMaxHeight`) continues to use `win.WorkArea()`.
3. **Top-Edge Attachment & Dragging**:
   - In `runWindow()`, `buildView()`, `redraw()`, `OnMove()`, and `reveal()`, Seam uses `seamArea := win.SeamWorkArea()` instead of `win.WorkArea()`:
     - Default top-center placement (`initialPillPosition(s, v, seamArea)`) places the visible rail flush at `y = seamArea.Min.Y` (`0`), horizontally centered in `seamArea`.
     - `v.Floating = a.y > seamArea.Min.Y` (`a.y > 0`). At `a.y == 0`, both the collapsed rail and the open overview render with `FillNotch` (`g.topPad = 0`, square/concave upper attachment at `y = 0`, `8 px` logical concave shoulders, `14 px` logical lower corners).
     - Dragging below `y = 0` (`a.y > 0`) transitions `v.Floating` to `true` (rounded floating pill/overview with `16 px` shadow padding on all four sides); dragging back to `y = 0` restores the attached notch contour.
4. **One-Time Settings Migration (`LayoutVersion: 2`)**:
   - In `seam_settings.go`, update `migrateSeamSettings(s settings) settings`:
     - If `s.LayoutVersion < 1`: clear `s.Compact = false`.
     - If `s.LayoutVersion < 2`: clear `s.HasPos = false` and set `s.LayoutVersion = 2` (retaining `s.Compact`, `s.Selected`, `s.Visible`, and `s.Docked` for users upgrading from `LayoutVersion == 1`).
     - This ensures existing installs whose Seam position was previously saved at `work.Min.Y` (`y = 30`) re-anchor to `y = 0` top-center once on upgrade, while preserving any subsequent drag positions saved under `LayoutVersion == 2`.

## 3. Z-Order & Input Coexistence Over Top Bars

1. **Same-Band Topmost Coexistence**:
   - `MyFinderApp` and `AgentQuota` both reside in `ZBID_DESKTOP` (band 1) with `WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE`.
   - In `internal/win/win.go`, add a 2-second timer (`tTopmost = 4`, armed in `StartPoll()` or `Create()`) that invokes `RaiseTopmost()` (`SetWindowPos(hwnd, hwndTopmost, 0, 0, 0, 0, swpNomove|swpNosize|swpNoactivate)`) when `hwnd != 0`, the window is currently visible (`windowVisible`), and no context menu is open (`!menuOpen`).
   - Do **not** include `swpShowwindow` in `RaiseTopmost()` so hidden windows (e.g., during fullscreen or when docked in the taskbar) are never accidentally shown.
2. **Click-Through Outside the Notch Contour**:
   - Keep `UpdateLayeredWindow` (`ULW_ALPHA`) + `WM_NCHITTEST` returning `HTTRANSPARENT` (`^uintptr(0)`) when `!cb.ContainsPoint(x, y)` (`raster.NotchContains`).
   - Concave shoulders, rounded bottom corners, and the 16 logical px shadow margins reject hits synchronously, allowing clicks immediately beside the notch to reach MyDockFinder's bar.

## 4. Fullscreen Detection & Suppression

1. **Detection (`win.Fullscreen() bool` in `internal/win/win.go`)**:
   - Return `true` when either of the following holds:
     a. **Shell notification state**: `SHQueryUserNotificationState(&state)` (`shell32.dll`) succeeds and `state` is `QUNS_BUSY` (`2`), `QUNS_RUNNING_D3D_FULL_SCREEN` (`3`), or `QUNS_PRESENTATION_MODE` (`4`), AND `isPrimaryMonitorForeground()` is true (or no foreground window on a secondary monitor contradicts it).
        *Refinement for reliability*: Because `SHQueryUserNotificationState` can return `QUNS_BUSY` (`2`) even when a non-fullscreen window or presentation setting is active if we are not careful, verify how `SHQueryUserNotificationState` behaves:
        - `state == 3` (`QUNS_RUNNING_D3D_FULL_SCREEN`) or `state == 4` (`QUNS_PRESENTATION_MODE`) is unambiguously fullscreen/presentation.
        - For `state == 2` (`QUNS_BUSY`) or any borderless fullscreen window (even if `SHQueryUserNotificationState` returns `5`), check `isForegroundFullscreenOnPrimary(hwnd)`:
          - Let `fg := GetForegroundWindow()`.
          - Must be non-zero, `fg != uintptr(hwnd)`, `IsWindowVisible(fg) != 0`, `IsIconic(fg) == 0`.
          - Not DWM-cloaked (`DwmGetWindowAttribute(fg, 14, &cloaked, 4)` has `cloaked == 0`).
          - Extended style (`GetWindowLongPtrW(fg, GWL_EXSTYLE)`) must not have `WS_EX_TOOLWINDOW` (`0x80`) or `WS_EX_NOACTIVATE` (`0x08000000`).
          - Class name (`className(fg)`) must not be a shell/desktop surface: `"Progman"`, `"WorkerW"`, `"Shell_TrayWnd"`, `"Shell_SecondaryTrayWnd"`, `"XamlExplorerHostIslandWindow"`, `"Windows.UI.Core.CoreWindow"`, `"MultitaskingViewFrame"`, `"ForegroundStaging"`.
          - Window rectangle (`GetWindowRect(fg)`) must cover ` mon := MonitorBounds()` (`r.Min.X <= mon.Min.X && r.Min.Y <= mon.Min.Y && r.Max.X >= mon.Max.X && r.Max.Y >= mon.Max.Y`), and if `state != 2`, style (`GetWindowLongPtrW(fg, GWL_STYLE)`) must not have `WS_CAPTION` (`0x00C00000`) so ordinary maximized titled windows on an auto-hide taskbar are never mistaken for fullscreen.
2. **Suppression Lifecycle in `app`**:
   - Add `fullscreen bool` check in `app.redraw()` (and on the 2-second timer in `internal/win/win.go` which notifies `cb` or triggers `OnMenu(menuTopmost)`):
   - When `!a.docked` and `win.Fullscreen()` is `true`:
     - If `a.open && !a.pinned`, set `a.open = false` and `a.hovered = widget.Target{}`.
     - Disable keyboard navigation (`win.EnableAgentNavigation(false)`).
     - Hide the window (`win.SetVisible(false)`) and return before `win.Resize` / `win.Flush`.
   - When `!a.docked` and `win.Fullscreen()` is `false`:
     - Ensure `win.SetVisible(true)` (or `win.Resize` which passes `swpShowwindow`) restores the window without activation.

## 5. Verification Plan

1. **Unit & Regression Tests**:
   - `seam_settings_test.go`: Verify `LayoutVersion < 2` migrates `HasPos = false` while keeping `Compact`, `Selected`, `Visible`, and `Docked`; verify `seamWorkArea` lifts `Min.Y` to `0` while keeping horizontal/bottom `WorkArea` bounds.
   - `seam_detach_test.go` & `main_test.go`: Verify top-edge attachment at `SeamWorkArea().Min.Y` (`0`), drag detachment into floating pill (`y > 0`), return to `y = 0` notch, and second-launch reveal within `SeamWorkArea()`.
   - `internal/win/win_test.go`: Verify `MonitorBounds()`, `SeamWorkArea()`, and `Fullscreen()` detection with a real borderless fullscreen popup window on the primary monitor vs. a normal titled window and a `WS_EX_TOOLWINDOW` window.
2. **Formatting, Vet, and Full Suite**:
   - `gofmt -l .` (empty), `go vet ./...`, `go test ./... -count=1`.
3. **Live Installation & Desktop Verification**:
   - Back up the currently installed `%LOCALAPPDATA%/Programs/AgentQuota/agentquota.exe` and `%APPDATA%/agent-quota/settings.json`.
   - Build and install `agentquota.exe`, launch it, and verify via Win32 inspection that the live `AgentQuota` window sits at `y = 0` (`(x, 0)-(x+w, h)`), above `MyFinderApp` (`0x620A7E`) in Z-order, with ` DESIGN.md` and `HANDOFF.md` updated.
