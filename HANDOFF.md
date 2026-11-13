# Agent Quota handoff

Updated: 2026-10-21. Top-edge Seam is implemented, installed and running.

## Latest correction

Seam now attaches to the primary monitor's top edge instead of the work area's top edge, so it draws over MyDockFinder's top bar (or a top-docked taskbar) as one continuous notch. `win.MonitorBounds`, `win.SeamWorkAreaFor`/`win.SeamWorkArea` lift only the top ceiling from `rcMonitor`, keeping the work area's sides and bottom, so side and bottom taskbars stay clear; Taskbar Focus still uses `win.WorkArea()`. A one-time `LayoutVersion 2` migration clears only `HasPos`, so an anchor saved at the old `y = 30` re-snaps to `y = 0` while Compact, Selected, Visible and Docked survive. A new 2 s timer (`tTopmost = 4`, delivered as `OnMenu(104)`) re-asserts topmost above same-band appbars with `SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE` — never `SWP_SHOWWINDOW`, so it cannot undo a deliberate hide — and hides Seam while `win.Fullscreen()` reports a borderless fullscreen app on the primary monitor, restoring it without activation afterwards. Fullscreen detection rejects DWM-cloaked windows, `WS_EX_TOOLWINDOW`, `WS_EX_NOACTIVATE`, `WS_CAPTION` and the shell/desktop classes, so a maximised titled window or MyDockFinder is never mistaken for fullscreen. Live: HWND 0x60714 at physical `(870,0)-(1058,68)`, `Top = 0`, visible, and above `MyFinderApp` (Z-order 14 vs 15) after an explicit appbar raise was corrected by the tick. `gofmt`, vet and the nine-package uncached suite pass. Installed executable SHA-256 `AB5BA6E5F290BCE5B03609D92B01D4A9FA8FF8DCFF2E1AC81D0A756D3371FB44`; rollback in `.debug-launch/top-edge-seam-20261021/rollback/`. See docs/verification/top-edge-seam/RESULT.md.

## Previous corrections

Left and Right now rotate the selected displayed agent while the pointer is over an open panel. Selection wraps, advances once per press, preserves pin/open state, resets old detail state, and persists. Outside, modified, hidden-panel and menu arrows retain their normal behavior. A dedicated Go hook thread preserves NOACTIVATE and posts generation-tagged selection commands to the UI thread. Immutable presented alpha masks exclude shadows. Native SendInput tests verify the actual hook with a protected receiver, including key balance and unchanged foreground. See docs/verification/keyboard-navigation/RESULT.md for choices, tests, and current installation hashes. Native host 68843005 is unchanged. The root checkout remains authoritative.

The user clarified that their actual complaint was slow, laggy dragging when removing the readout from the taskbar. That input backlog is corrected and installed. Native mouse movement was forwarded as ordinary posted messages, so Go replayed obsolete positions instead of using Windows' normal latest-position behavior. A real-window reproduction took 388.8 ms to drain 120 queued moves; after coalescing adjacent moves it takes 3.5 ms. Release, capture cancellation, geometry events and connection-generation boundaries retain their order. The deterministic queue regression failed before the fix and passes after it; full formatting/vet/nine-package tests and the opt-in real Explorer integration pass. The drag-only build used executable hash 7A253601A928E1430C367D42235C877EA563E1A0945BE1B9E5F3DEA8C87754E1. The keyboard feature above supersedes that executable. Native host 68843005 is unchanged. See docs/verification/taskbar-focus/production/drag-latency/RESULT.md. Physical pointer smoothness could not be observed because the native automation pipe remains disconnected after retry/reset recovery. The earlier ownership correction below addressed a separate bug, not the user's stated lag symptom.

The duplicate readout in the user's screen recording is fixed and installed. A frame-size change invalidated native fit, then restored it without notifying Go when Explorer retained the same allocation. Go kept its floating fallback while the native readout resumed painting. The host now acknowledges completion even when the allocated rectangle stays unchanged. A retained-allocation regression failed before the fix and passes after it; a real Go-to-Explorer integration test passes 12 alternating frame-size changes plus floating/docked handoffs. Current host: 68843005 (Host3 identity, protocol 2). Evidence: docs/verification/taskbar-focus/production/duplicate-readout/RESULT.md. The user's original floating preference was restored after testing; a collapsed native slot in final-state.json is expected. Explorer PID10872 was preserved. Root checkout is authoritative for this correction; the earlier isolated implementation worktree has not been refreshed.

The Focus popup now includes independent usage bars beneath each reported five-hour/weekly percentage, as requested in the annotated screenshot. Bars follow the same green/amber/red thresholds as the rings, retain a dimmed last-good fill when stale, and leave zero/unknown usage unfilled. This update is installed; current hashes are in production/final-install.json, preview/state evidence in production/pool-bars/.

The user clarified that the docked readout must blend with the taskbar instead of using a black tile. This is fixed and installed. widget.RenderTaskbar emits transparent premultiplied pixels; the native host paints Explorer's themed parent background before alpha blending the content. Popup and floating surfaces stay black. Protocol 2 originally shipped as host 3e325385; the correction above supersedes that build and retires both earlier COM identities without restarting Explorer. Current DLL and executable hashes are in production/final-install.json. Background-specific evidence is in production/background-fix/.

## Current result

The user's instruction to finish the remaining work authorized the production native deskband and installation. Do not repeat the design interview or ask them to choose integration routes. The selected Focus structure is settled.

The app now uses a real Explorer-allocated slot beside the tray on the primary bottom Windows 10 taskbar. The complete two-line readout remains visible. Clicking toggles the Focus detail popup; the selected agent drives both surfaces. Floating/top-edge Seam remains available. Docking saves its own preference separately from the floating anchor.

Go owns readers, credentials, settings, rendering and the popup. The small C++ DLL owns only bounded premultiplied pixels, input, and shell sizing. There is no cgo, browser runtime, additional quota polling loop or credential cache. The accepted cadence remains local/API 30 seconds, Kiro/Antigravity CLI five minutes, immediate manual refresh and one in-flight read per agent.

## Read these authoritative files

- Project rules: AGENTS.md and the user's conversation instructions.
- Shipped behavior and limits: DESIGN.md and docs/verification/taskbar-focus/verification.md.
- Build/registration/diagnostics: native/taskbar/README.md.
- Exact installed hashes/processes: docs/verification/taskbar-focus/production/final-install.json.
- Native protocol/lifecycle results and measured production geometry: docs/verification/taskbar-focus/production/.
- Approved design and original acceptance scope: docs/superpowers/specs/2026-10-17-taskbar-focus-design.md and docs/superpowers/plans/2026-10-17-taskbar-focus.md. Later implementation authorization supersedes their earlier pure-Go/probe-only stop.

## Verified and remaining evidence limits

Formatting, vet, the uncached nine-package Go suite, nine native parser checks and 27 native COM/lifecycle/paint checks pass. The opt-in TestLiveTaskbarFrameOwnership integration test passed against the installed Explorer host. Renderer fixtures cover all seven agents, absent/zero/unlimited/stale/failure data, scrolling, and 100/125/150/200 percent scales.

Live checks confirm a complete 148x48 frame inside the shell's 152x48 slot at 125 percent DPI, task-button reflow without overlap, unchanged 50-pixel taskbar height, owner-exit collapse, new-generation relaunch without another enabling prompt, foreground-preserving second-instance reveal, auto-hide popup suppression and restoration. The installed executable and native DLL match their recorded hashes. Windows' initial toolbar confirmation was accepted through the normal Yes action.

Physical pointer drag/click, live monitor DPI transitions, crowded-taskbar stress and an actual Explorer restart remain unobserved. The automation tool does not expose the Focus toolwindow or taskbar child as targetable surfaces, and native screenshots timed out in the earlier probe. Do not misstate renderer/COM tests as those live acceptance checks. Explorer was not restarted automatically under the existing execution constraint. The implementation supports only the tested primary bottom Windows 10 arrangement; unsupported layouts and insufficient space/height fall back to floating mode.

## Installation and rollback

Installed executable: %LOCALAPPDATA%/Programs/AgentQuota/agentquota.exe.
Current installed executable SHA-256 AB5BA6E5F290BCE5B03609D92B01D4A9FA8FF8DCFF2E1AC81D0A756D3371FB44. Its preceding executable and UI settings are backed up in .debug-launch/top-edge-seam-20261021/rollback/ (executable SHA-256 E651DCE4C666186E53A4EFE690D9CE6BDAA3081AF0C8DD6FA20CD2BF721D2BAF).
Native host: agentquota-taskbar-68843005.dll with taskbar-host.json in the same folder.
The immediately previous executable, manifest and UI settings are backed up in .debug-launch/recording-debug-20261017/rollback/. Older mapped DLLs remain in place until Explorer releases them.
The drag-backlog correction additionally backs up its preceding installed/root executables and settings in .debug-launch/drag-latency/.
The keyboard feature's preceding executables and settings are in .debug-launch/keyboard-navigation/. Its current installed hashes are in docs/verification/keyboard-navigation/final-install.json.
Rollback executable and UI settings: .debug-launch/focus-rollback-20261017-030251/ in this checkout. Credentials were not copied or changed. Stopping Go releases the native slot automatically; restoring the rollback executable/settings restores the previous app. The enabled disconnected band can be disabled through Windows' Toolbars menu.

Use --taskbar-status for credential-free native geometry. A separate diagnostic process has its own generation: native_fits is the current host result; snapshot.Fits is intentionally false for an unattached diagnostic caller.

## Workspace preservation

Implementation was isolated in .debug-launch/focus-worktree on feat/taskbar-focus-native, seeded with the complete dirty native source rather than stale HEAD. The owned production files were integrated back into this checkout after baseline hash checks. The original preexisting native modifications remain. No broad staging, reset, clean or commit was performed. Do not discard the uncommitted native work.

The earlier disposable-probe sources/evidence are historical feasibility diagnostics, not the installed runtime. Production source is native/taskbar/, internal/win/taskbar_windows.go, taskbar.go, widget/focus.go and their tests. Main-agent implementation and verification remain the project rule; bounded helpers are read-only.
