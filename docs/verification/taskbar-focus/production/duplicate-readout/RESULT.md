# Duplicate readout correction — 2026-10-17

The supplied recording showed a floating pill above the taskbar while the native taskbar readout remained visible. Dragging the floating copy left the native copy behind until the final undock. The corrected host is installed and loaded in Explorer without restarting it.

## Cause and correction

`Band::Receive` clears `AQ.Fits` when a frame changes size, then defers shell negotiation. Go observes the cleared fit and displays its floating fallback. Native `Geometry` restores fit after negotiation, but previously deduplicated its notification against the last reported rectangle, DPI and fit. When Explorer retained the same allocation, all three matched and Go received no completion event. The native copy resumed painting while Go retained the floating fallback.

A retained allocation is permitted: Microsoft's [DESKBANDINFO documentation](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/ns-shobjidl_core-deskbandinfo) describes `ptActual` as an ideal size, and horizontal maximum width is ignored. The existing native connection path already invalidates the reported-DPI sentinel to force a completion notification. The changed-frame path now does the same before deferred layout. No drag policy or provider behavior changed.

The native regression keeps a 148-pixel client allocation while accepting a 140-pixel frame, then counts actual geometry messages delivered to the owner:

```text
Before: invalidated=1 restored=1 geometry_notifications=0  FAIL
After:  invalidated=1 restored=1 geometry_notifications=1  PASS
```

## Validation

- `gofmt -l .`: no output; `go vet ./...`: pass.
- `go test ./... -count=1 -timeout 90s`: all nine packages pass. See [go-tests.txt](go-tests.txt).
- Native build uses `/W4 /WX`; 27 host/lifecycle/paint checks pass, including the retained-allocation regression. See [native-regression.txt](native-regression.txt).
- Nine packet-parser checks pass. See [protocol-tests.txt](protocol-tests.txt).
- `AGENTQUOTA_LIVE_TEST=1 go test . -run '^TestLiveTaskbarFrameOwnership$' -count=1 -v -timeout 45s`: pass against the real enabled Explorer host in 0.83 seconds. Deterministic fixture readings invoke no provider or credentials. Twelve alternating one/two-pool changes finish with native fit and the floating window hidden. Floating handoff collapses native width to zero; docking restores fit and hides the floating copy. See [live-test.txt](live-test.txt) and [live-frame-ownership.json](live-frame-ownership.json).

The integration test checks app-to-Explorer ownership and mode transitions. It does not automate physical pointer dragging. The native automation pipe failed its documented retry/reset recovery, so the user accepted Windows' new-toolbar confirmation manually. Live monitor DPI changes, crowded-taskbar stress and Explorer restart remain unobserved.

## Installed result

Host3 uses protocol 2 and build `68843005` (`1753493509`), with a new COM/window identity so Explorer loads the corrected DLL rather than its cached factory. The installer retired both earlier Agent Quota identities. Old DLL files remain in place until Explorer releases them. Explorer PID `10872` was unchanged.

Installed executable and DLL hashes match the tested build and manifest; the loaded host reports the expected build. Exact paths and hashes are in [final-install.json](final-install.json). The user's original floating preference and exact saved settings were restored after the integration test. [final-state.json](final-state.json) therefore records one visible floating window, a hidden native readout and zero native width; `native_fits=false` is expected in this mode. Previous executable, manifest and settings are backed up under `.debug-launch/recording-debug-20261017/rollback/`.

The original user recording and extracted desktop frames stay in local ignored storage rather than published verification artifacts. The root checkout is authoritative for this correction; no broad staging, reset, clean or commit was performed.
