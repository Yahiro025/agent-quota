# Taskbar drag backlog correction — 2026-10-17

The user clarified that the reported issue was slow movement while dragging the readout out of the taskbar. The earlier ownership fix addressed a separate bug. This correction targets input latency and is installed in both the normal installation and the root executable.

## Investigation

Native `Band::WindowProc` receives captured mouse movement in Explorer, then `Notify(5, point)` forwards each position through a registered `PostMessage` to Go. Go's `handleTaskbarMessage` previously dispatched every forwarded point to `app.OnTaskbarEvent`, which redraws the floating drag representation. Unlike ordinary Windows mouse movement, these custom posted messages were not coalesced. Microsoft's [Mouse Input Overview](https://learn.microsoft.com/en-us/windows/win32/inputdev/about-mouse-input) describes the system's latest-message behavior when ordinary mouse input arrives faster than it can be processed.

Initial callback profiling found native drag updates averaged 3.89 ms versus 1.50 ms for floating moves, with native p95 7.41 ms. That alone did not reproduce a large persistent lag. Testing the actual forwarded-message queue did: 120 queued positions took 388.78 ms to reach the newest point. Each obsolete point caused another redraw. CPU profiling also showed substantial Win32 geometry/query work in `TaskbarBridge.Snapshot`; raster rendering was only part of the per-event cost. No renderer rewrite was justified to fix the backlog.

## Minimal correction

`internal/win/taskbar_windows.go` drains adjacent pending movement messages into the latest point before dispatching one drag callback. It stops at release, cancellation, geometry or any other taskbar event, and at another connection generation. These boundaries remain in order. Native code, protocol and host identity do not change.

The deterministic real-message-queue regression, `TestTaskbarDragDropsBacklogWithoutCrossingInputBoundaries`, originally delivered 129 callbacks rather than nine latest-position/control events. It now delivers the expected events, including signed coordinates, exact release position, capture cancellation and generation separation. See [queue-red.txt](queue-red.txt) and [queue-green.txt](queue-green.txt).

## Live-window measurements

The opt-in `TestLiveTaskbarFrameOwnership` uses the actual Go layered window and the enabled Explorer host with deterministic quota fixtures. `AGENTQUOTA_DRAG_EVIDENCE` enables the drag measurements. It invokes the production drag callbacks and posts the same registered movement messages used by the native host; it does not move the user's physical mouse or call providers.

| Measurement | Before | After |
|---|---:|---:|
| Time to drain 120 queued movement points | 388.78 ms | 3.46 ms |
| Mean native drag callback | 3.89 ms | 3.91 ms |
| p95 native drag callback | 7.41 ms | 7.29 ms |
| Mean ordinary floating move | 1.50 ms | 1.51 ms |

The change removes obsolete work, rather than making an individual redraw faster. Raw records: [before.json](before.json), [after.json](after.json). The integration verifies the newest point retains the original 30x20 pointer offset, all 12 one/two-pool frame changes retain correct ownership, and floating/docked handoffs still work. Its backlog performance gate allows 50 ms for OS scheduling; exact queue behavior is covered by the normal deterministic test.

`gofmt -l .` has no output; `go vet ./...` and all nine packages in `go test ./... -count=1 -timeout 90s` pass. The Windows GUI build succeeds. See [go-tests.txt](go-tests.txt) and [after-test.txt](after-test.txt).

Physical pointer smoothness was not observed: native Computer Use reports a missing native pipe after its lightweight retry and reset/reinitialize recovery. No custom mouse-injection helper was substituted. The measured result establishes correction of the reproduced input backlog, not a visual physical-drag rehearsal.

## Installation

Installed `%LOCALAPPDATA%/Programs/AgentQuota/agentquota.exe` and root `agentquota.exe` match SHA256 `7A253601A928E1430C367D42235C877EA563E1A0945BE1B9E5F3DEA8C87754E1`. Native host `68843005` remains loaded, with its existing verified DLL hash. Explorer PID10872 was preserved. [final-install.json](final-install.json) records exact paths/processes; [final-state.json](final-state.json) confirms the active floating window and collapsed native slot. The latest saved floating position is preserved.

Previous installed/root executables and pre-test UI settings are backed up in `.debug-launch/drag-latency/`. The root checkout remains authoritative. No staging, reset, clean or commit was performed.
