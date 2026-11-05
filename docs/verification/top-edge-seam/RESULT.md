# Top-edge Seam over top bars

Spec: [2026-10-21-top-edge-seam-design.md](../../superpowers/specs/2026-10-21-top-edge-seam-design.md)

Seam now attaches to the primary monitor's top edge instead of the work-area's top edge, so it draws over a top appbar such as MyDockFinder (`MyFinderApp`, `(0,0)-(1920,30)` physical at 125% DPI) or a top-docked taskbar as one continuous notch. A two-second maintenance tick keeps it above same-band appbars and hides it while a borderless fullscreen app owns the primary monitor. Taskbar Focus still uses the plain work area.

## Implementation choices

| Design question | Decision |
|---|---|
| Replace `WorkArea()` everywhere | Rejected. Taskbar Focus popups and `FocusMaxHeight` must stay inside the work area; only Seam placement, clamping and attachment use the new rectangle. |
| Clamp Seam to `MonitorBounds()` | Rejected. That would let Seam cover a side or bottom taskbar. `SeamWorkAreaFor` lifts only the top ceiling from the monitor and keeps `work.Min.X`, `work.Max.X` and `work.Max.Y`. |
| Include `SWP_SHOWWINDOW` in the maintenance raise | Rejected. The tick must never undo a deliberate hide (docked readout, fullscreen suppression), so the raise uses `SWP_NOMOVE \| SWP_NOSIZE \| SWP_NOACTIVATE` only. |
| Trust `SHQueryUserNotificationState` alone | Rejected. It can report busy for reasons unrelated to fullscreen, and can report normal state for a borderless fullscreen window. It is combined with a foreground-window shape/class/style test, and only `QUNS_RUNNING_D3D_FULL_SCREEN` and `QUNS_PRESENTATION_MODE` are accepted from the shell state. |
| Treat every monitor-covering foreground window as fullscreen | Rejected. Excluding `WS_CAPTION`, `WS_EX_TOOLWINDOW`, `WS_EX_NOACTIVATE`, DWM-cloaked windows and shell/desktop classes keeps an ordinary maximised window on an auto-hide taskbar, MyDockFinder and the desktop from being mistaken for fullscreen. |
| Re-centre on every redraw | Rejected. The position is the user's saved anchor; only the one-time `LayoutVersion 2` migration clears it. |

## Geometry

`win.MonitorBounds()` queries `MonitorFromPoint({0,0}, MONITOR_DEFAULTTOPRIMARY)` and `GetMonitorInfoW` (`cbSize = 40`), returning `rcMonitor` in physical pixels and falling back to `ScreenSize()`.

`win.SeamWorkAreaFor(work, monitor)` returns `image.Rect(work.Min.X, monitor.Min.Y, work.Max.X, work.Max.Y)` when the monitor's top edge is above the work area's, else `work`. `win.SeamWorkArea()` is that call with `WorkArea()` and `MonitorBounds()`.

Measured on the live desktop (per-monitor-v2 DPI, physical pixels):

```
monitor=(0,0)-(1920,1200)  work=(0,30)-(1920,1200)  seam=(0,0)-(1920,1200)
```

Under MyDockFinder the work area starts at `y = 30`; the Seam area starts at `y = 0` with the same sides and bottom.

`runWindow`, `buildView`, `redraw`, `OnMove` and `reveal` use `win.SeamWorkArea()`, so `v.Floating` is `a.y > 0` and the notch contour applies at `y = 0`. `win.WorkArea()` still drives `v.FocusMaxHeight` and `taskbar.go`'s `focusPopupPosition`.

## Settings migration

`migrateSeamSettings` gains a second one-time step. `LayoutVersion < 1` still clears `Compact` and the saved position. `LayoutVersion < 2` clears only `HasPos` and records version 2, so an anchor stored at the old work-area top edge (`y = 30`) re-snaps to `y = 0` while `Compact`, `Selected`, `Visible` and `Docked` survive. Version 2 positions are never touched again.

The live settings file went from `layout_version: 1, x: 909, y: 30` to `layout_version: 2, x: 890, y: 0` with `compact: true`, `selected: "claude"` and the same visibility map.

## Z-order and fullscreen

`windowVisible` mirrors `ShowWindow` state (`Create`, `Resize`, `SetVisible`), and `menuOpen` brackets `showMenu`. `win.RaiseTopmost()` returns early when the window is unknown, hidden or behind an open menu; otherwise it calls `SetWindowPos(hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)`.

`win.StartPoll()` arms `tTopmost + 100` (`104`) at 2 s beside the existing 30 s poll tick. `app.OnMenu(104)` skips docked and drag states, converts a `Fullscreen()` transition into one redraw (collapsing an unpinned overview and clearing hover on entry), and otherwise calls `RaiseTopmost` while not fullscreen. `app.redraw()` hides the window before sizing whenever `a.fullscreen` is set, and the restore path reuses `Resize`'s `SWP_SHOWWINDOW|SWP_NOACTIVATE`.

`win.FullscreenFor(fg)` rejects `fg == 0`, `fg == hwnd`, invisible and iconic windows, DWM-cloaked windows (`DwmGetWindowAttribute(14)`), `WS_EX_TOOLWINDOW`/`WS_EX_NOACTIVATE`, `WS_CAPTION`, the shell/desktop classes `Progman`, `WorkerW`, `Shell_TrayWnd`, `Shell_SecondaryTrayWnd`, `XamlExplorerHostIslandWindow`, `Windows.UI.Core.CoreWindow`, `MultitaskingViewFrame` and `ForegroundStaging`, then requires the window rectangle to cover `MonitorBounds()`. `win.Fullscreen()` combines that with `SHQueryUserNotificationState` states 3 and 4.

## Tests

| Test | Covers |
|---|---|
| `TestSeamMigrationRunsOnceAndRetainsReaderPreferences` | version 0 clears position and compact; version 1 clears only the position and keeps compact, selection, visibility and docking at version 2; version 2 keeps the user's drag |
| `TestSeamWorkAreaReachesMonitorTopEdge` | `(0,30,1920,1200)` + `(0,0,1920,1200)` yields `(0,0)-(1920,1200)`, a bottom taskbar still bounds it, an empty monitor falls back to the work area, and `initialPillPosition` puts the notch at `y = 0` for 100/125/150/200% |
| `TestSeamPositionSelectsAttachedOrFloatingContour` | attachment and floating contours measured against the Seam area |
| `TestSeamDragChangesContourWithoutMovingItsAnchor` | real-window drag from `y = 0`, detach, return to the notch, reveal and save |
| `TestFullscreenTickHidesAndRestoresSeamWithoutActivation` | `OnMenu(104)` hides a visible real window on a fullscreen transition, keeps it hidden on a steady-state tick, and restores it with the foreground window unchanged |
| `TestFullscreenForDistinguishesBorderlessFullscreenFromTitledAndToolWindows` | three real Win32 windows covering `MonitorBounds()`: captionless `WS_POPUP` is fullscreen, `WS_OVERLAPPEDWINDOW` and `WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE` are not |
| `TestRelaunchRevealsExistingWidgetInsideWorkArea` | second-launch reveal stays inside the Seam area |

The three probe windows are `WS_EX_LAYERED` with alpha 0 so the desktop is not visibly covered during the run; they are still genuinely visible, monitor-sized and uncloaked.

## Verification

```powershell
gofmt -l .            # no output
go vet ./...          # clean
go test ./... -count=1
```

```
ok  	agent-quota	4.497s
ok  	agent-quota/internal/core	1.574s
ok  	agent-quota/internal/marks	0.763s
ok  	agent-quota/internal/proc	1.911s
ok  	agent-quota/internal/raster	0.721s
ok  	agent-quota/internal/text	1.109s
ok  	agent-quota/internal/win	0.847s
ok  	agent-quota/quota	7.549s
ok  	agent-quota/widget	3.839s
```

`go run . --shot DIR` writes 217 PNGs and exits 0. `go run . --print` reads all seven agents and exits 0.

### Live desktop

Rollback and install:

| Item | Value |
|---|---|
| Rollback executable | `.debug-launch/top-edge-seam-20261021/rollback/agentquota.exe` |
| Rollback settings | `.debug-launch/top-edge-seam-20261021/rollback/settings.json` |
| Previous executable SHA-256 | `E651DCE4C666186E53A4EFE690D9CE6BDAA3081AF0C8DD6FA20CD2BF721D2BAF` |
| Installed executable SHA-256 | `AB5BA6E5F290BCE5B03609D92B01D4A9FA8FF8DCFF2E1AC81D0A756D3371FB44` |
| Installed size | 17,536,000 bytes |

Before the install the running window sat at `(711,24)-(861,78)` in DPI-unaware coordinates, i.e. `y = 30` physical — the work area's top edge, below MyDockFinder's 24-logical-pixel bar.

After the install, measured by a per-monitor-v2 process (physical pixels, 1920x1200 at 120 DPI):

```
HWND          0x60714
PhysicalRect  870,0 - 1058,68
Top           0
Visible       True
CentreOffset  4 px from the monitor's horizontal centre
```

A DPI-unaware probe of the same window reports `(696,0)-(846,54)` in a 1536x960 virtual desktop, which is the same `Top == 0` scaled by 125%. `EnumWindows` Z-order indices place the widget above the appbar:

```
before:                 agent=14  MyFinderApp=22
MyFinderApp raised:     agent=15  MyFinderApp=12
after 3 s (untouched):  agent=14  MyFinderApp=15
```

Raising `MyFinderApp` to the top of the topmost band put it above the widget; the next 2 s maintenance tick restored the widget above it without moving, resizing, showing or activating anything. The foreground window was unchanged throughout.

The window's extended style is `0x08080088` (`WS_EX_TOPMOST|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE|WS_EX_LAYERED`); `MyFinderApp` is `0x08200088`, which the fullscreen test excludes.

### Evidence limits

- The fullscreen hide/restore path is verified by the real-window tick test and the real-window `FullscreenFor` test. It was not exercised against a live fullscreen game or video player: doing so requires handing the foreground to another process, which would steal focus from the user's session.
- Physical mouse dragging, click-through beside the notch and live DPI transitions were not re-observed in this round; their code paths are unchanged apart from the rectangle they clamp to, and the existing real-window drag test passes.
- The notch sits 4 physical pixels right of the exact horizontal centre because the first paint estimates the rail width before the readings arrive; the saved anchor is not re-centred afterwards by design.

## Rollback

Stop `agentquota`, copy `rollback/agentquota.exe` and `rollback/settings.json` back to `%LOCALAPPDATA%\Programs\AgentQuota\agentquota.exe` and `%APPDATA%\agent-quota\settings.json`, then launch the executable. Credentials were not read, copied or changed.
