# Native deskband feasibility probe

Date: 2026-10-17. **Placement mechanism demonstrated on this Windows 10 machine; full Focus integration gate remains incomplete.**

The user authorized the native probe after the architecture review. The probe was built, temporarily registered per user, hosted by Explorer, measured, and removed. It was not installed as the product. The original pure-Go/out-of-process verdict remains FAIL under those original constraints.

## What ran

- Native C++ static deskband plus disposable controller, built with the installed VS 2022 Build Tools and Windows SDK. `/W4 /WX` build passes. No toolchain or runtime dependency was installed.
- Go generated frozen readout and selected-detail popup resources through the existing `widget.Render`. The readout preserves the authentic Codex mark, separate 42% five-hour / 78% weekly rings, primary percentage and `5h 1h24m` reset line. These are fixtures, not account readings.
- Seventeen isolated native checks passed: frame validation, all four resource scales, COM activation, child attachment, native sizing, title suppression, mouse-activation refusal and clean release/unload outside Explorer. See [selftest.txt](selftest.txt).
- `go vet ./...` and `go test ./... -count=1` passed for all nine packages. `gofmt -l .` produced no output. No product source was changed by this probe.

Source and reproduction: [deskband-probe](../../../scratch/taskbar-focus/deskband-probe/README.md). Binary identities are in [binary-hashes.json](binary-hashes.json).

## Native placement evidence

Machine: Windows 10 Pro 22H2, 19045.6466, primary display at 120 DPI (125%).

| Region | Before | Probe enabled |
| --- | --- | --- |
| Actual taskbar window | `[0,1150,1920,1200]` | unchanged |
| Primary work area | `[0,0,1920,1150]` | unchanged |
| Task-button container | `[120,1150,1545,1200]` | `[120,1150,1387,1200]` |
| Native probe child | absent | `[1393,1151,1545,1199]` |
| Tray | `[1545,1150,1920,1200]` | unchanged |
| Frozen readout | 148 × 48 physical pixels | fits a 152 × 48 client rectangle |

Explorer allocated the band inside its rebar and reduced the task-button container. The child fits within both ancestors, does not intersect task buttons or tray, and leaves the taskbar at 50 pixels high. The probe did not resize or reparent any preexisting Explorer child.

See [enabled-geometry.json](enabled-geometry.json), [cycle-reenabled.json](cycle-reenabled.json), and [checks.json](checks.json). Explorer PID 10872 emitted class-factory, attachment, child-creation and size-reporting events in [events.log](events.log), establishing actual shell hosting rather than only external COM activation.

## Registration and enabling

The probe registers only its unique CLSID under `HKCU\Software\Classes`, with `InprocServer32`, `Apartment`, and `CATID_DeskBand`. No elevation, machine-wide registration, Approved-list changes or security-policy changes were used.

Initial attempts returned `E_FAIL` from `ShowDeskBand`. The final attempt verified discovery through `ICatInformation::EnumClassesOfCategories` and sent `SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST | SHCNF_FLUSHNOWAIT)` plus `DeskBandRegistrationChanged` after registration. `ShowDeskBand` then returned success and Windows presented its standard toolbar confirmation. The agent accepted that authorized probe confirmation through the native UI tool. The sequence succeeded; this is evidence for the complete sequence, not proof that the missing notification alone caused the earlier failures.

**`ShowDeskBand` returning `S_OK` is insufficient:** before the confirmation was accepted, `IsDeskBandShown` returned `S_FALSE` and there was no native child. The child and allocated geometry appeared after acceptance. Re-enabling after `HideDeskBand` produced the confirmation again; accepting it restored the same geometry.

This affects the selected interaction: silent, app-controlled drag-to-dock is not established by showing a deskband. Production needs an explicit initial enabling experience and a proven way to attach/detach without repeatedly invoking this confirmation, or a reviewed interaction revision. Do not bypass the Windows prompt.

## Auto-hide and popup

- Auto-hide was temporarily enabled and then restored, with independent outer cleanup. The actual taskbar moved from y=1150 to y=1198, and the probe moved with it from y=1151 to y=1199. The normal edge strip remained; the readout did not remain detached above the hidden taskbar. See [autohide.txt](autohide.txt).
- Hide/show was exercised. Hiding restored the original task-button rectangle. Re-enabling after the Windows confirmation restored the same band rectangle.
- Three synthetic popup toggles retained the foreground HWND: first `262882 → 262882`, then twice `11343424 → 11343424`. The source and event labels explicitly identify these as synthetic operations.
- The popup is a native C++ window displaying a frozen Go-rendered **existing Seam** image. It is not the unimplemented Focus renderer, a running Go popup, or a live frame-transport test.

## Verification limits

Native screenshot capture through the available tool failed with `FrameArrived timed out` / `window capture timed out`. Accessibility and keyboard input worked for the shell confirmation. The tool did not expose the hosted taskbar child as a targetable window, so a physical pointer click on the readout and continued typing in another application were not verified. The attached [readout125.png](readout125.png) and [popup125.png](popup125.png) are pure renderer fixtures, not live screenshots.

Also unverified: live DPI transitions, crowded taskbar behavior, shell dragging/undocking, saved-state restoration, Go absence/restart behavior and Explorer restart/recreation. Explorer was not restarted. Resources for 100/125/150/200% decode successfully, but only 125% was exercised in the real shell.

## Cleanup

The outer driver restored the appbar state, closed the disposable controller/popup, hid the band, unregistered the exact probe CLSID and verified the native child was absent. The taskbar/work-area/task-button geometry returned to baseline. The taskbar remained locked.

[before.json](before.json) and [after.json](after.json) show identical installed executable hash, quota-settings hash, Agent Quota PID and Explorer PID. The installed app was not stopped, rebuilt or replaced. No probe process remained. Diagnostic binaries are retained in the ignored `.debug-launch/deskband-probe/` folder; registration removal does not establish immediate DLL unloading by Explorer, so those files were not deleted.

## Disposition

**Use a minimal native deskband as the leading placement candidate.** Its actual shell slot and auto-hide behavior are demonstrated here. Keep quota acquisition, scheduling, CPU rendering and the eventual Focus popup in Go; keep the native adapter restricted to hosting/display/input.

Do not mark the full implementation gate PASS yet. Resolve the enabling/drag contract and obtain the missing real interaction/lifecycle evidence before claiming the selected Focus experience works or replacing the installed app.

## Primary references

- [Microsoft native DeskBand sample](https://github.com/microsoft/Windows-classic-samples/tree/main/Samples/Win7Samples/winui/shell/shellextensibility/deskbands): native COM hosting and registration structure.
- [Shell extension registration](https://learn.microsoft.com/en-us/windows/win32/shell/reg-shell-exts): shell change notification after registration.
- [ITrayDeskBand](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl/nn-shobjidl-itraydeskband): show, hide, query and registration-cache refresh.
- [ABM_SETSTATE](https://learn.microsoft.com/en-us/windows/win32/shell/abm-setstate): temporary auto-hide state and required fields.
- [IDeskBand reference](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nn-shobjidl_core-ideskband): unsupported-status warning remains relevant; this probe demonstrates machine-specific compatibility, not a supported forward-compatibility guarantee.
