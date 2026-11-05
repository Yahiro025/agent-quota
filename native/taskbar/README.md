# Native taskbar host

The optional x64 Windows 10 deskband reserves space through `IDeskBand2`; it never resizes Explorer's private task-button windows. Go owns credentials, quota readers, settings, rendering and the Focus popup. The DLL owns a premultiplied BGRA frame, shell sizing and pointer notifications. It has no network calls, quota timers, credential access, or child processes.

Build from the repository root with Visual C++ Build Tools and the Windows SDK installed:

```powershell
& native/taskbar/build.ps1
```

This runs the protocol and COM lifecycle tests, then builds the Go executable, a versioned DLL, and `taskbar-host.json` under `.debug-launch/focus-build`. Normal `go build .` remains independent of the C++ toolchain and falls back to floating mode when the host is unavailable.

Copy all three artifacts to the installation folder, then run:

```powershell
agentquota.exe --install-taskbar C:\path\to\taskbar-host.json
```

Registration is per user. The installer verifies the DLL hash, resolves COM category discovery, and requests the standard Windows toolbar confirmation. It cannot bypass that confirmation. Start Agent Quota after installation. Subsequent docking uses collapse/expansion of the enabled band, avoiding repeated enabling prompts. Keep mapped versioned DLLs in place until Explorer releases them. An upgrade that still has an older host loaded falls back to floating mode; it must not be treated as a successful native upgrade until `--taskbar-status` reports the expected build.

Use `agentquota.exe --taskbar-status` for geometry, build, fit and popup flags. `native_fits` is the host's current result; `snapshot.Fits` additionally requires the diagnostic caller's own connection generation, so a separate diagnostic process reports false there by design.

The tested arrangement is the primary, bottom, horizontal Windows 10 taskbar. Windows 11, top/vertical taskbars, insufficient height, missing hosts and mismatched builds use the floating widget. This uses a legacy shell extension interface; compatibility with other Windows builds is not promised.

`WM_COPYDATA` packets have a bounded, versioned 32-byte header and exact pixel length. The receiver copies pixels before acknowledging and defers shell relayout. HWND/PID/generation checks prevent connection confusion; they are not a security boundary against another process running as the same user. Notifications carry only geometry and pointer events. Owner destruction collapses the slot through a WinEvent hook. Hooks run on their installing apartment and keep the DLL loaded until removed.

To return to the previous app, stop Agent Quota, restore the saved executable and settings, then launch it. The disconnected band releases its width. Windows' Toolbars menu can disable the enabled band completely; do not delete a DLL while Explorer has it mapped.


Protocol 2 paints the shell-owned background using DrawThemeParentBackground and blends transparent premultiplied pixels over it with AlphaBlend. The docked readout has no black surface; the Go Focus popup and floating widget retain theirs. Theme/color notifications repaint the background. The current Host3 COM/window identity retires both previous host identities before enabling the corrected host, avoiding a cached old factory without restarting Explorer. Native paint tests verify transparent margins, antialias blending and unchanged opaque mark colors against a nonblack parent.

Each changed frame invalidates fit until deferred shell negotiation completes. Completion must notify Go even if Explorer retains the previous allocation, so the temporary floating fallback is dismissed. The native regression models that unchanged rectangle. The opt-in Go integration test `TestLiveTaskbarFrameOwnership` attaches to an enabled matching Explorer host and checks repeated frame-size changes and both mode handoffs. Stop the normal app before running it with `AGENTQUOTA_LIVE_TEST=1`; fixture readings do not invoke providers. Set `AGENTQUOTA_LIVE_EVIDENCE` to save its ownership samples. Restore the normal app after the test.

Go coalesces adjacent forwarded drag-move messages before painting, keeping the latest position rather than replaying a backlog. It stops at control events or a different connection generation. Set `AGENTQUOTA_DRAG_EVIDENCE` to a JSON path during the opt-in integration test to measure 120 native/floating callbacks and a 120-move message backlog against the actual Go window and Explorer connection. This does not move the physical mouse; the normal unit suite separately checks exact latest-position delivery and release/cancellation ordering.
