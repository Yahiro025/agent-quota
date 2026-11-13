# Native Orbit verification

The user selected A / Orbit in chat on 15 October 2026. The native Windows
application now uses that design. `DESIGN.md` describes the accepted tokens,
geometry and behavior. No new runtime dependencies were added.

## Result

- A 528 × 44 logical-pixel seven-agent rail and an independent 386 px inspector.
- A 112 × 38 compact tab, available from the right-click menu. The selected agent
  and compact preference are saved in the existing settings file.
- Separate Pin, Close, Refresh, reset, model-group and recovery targets. The
  renderer owns both drawing and hit geometry. Header controls do not start a
  drag; the pill and the unused header area can be dragged.
- Structured source windows retain short and weekly percentages separately from
  the maximum shown in the ring. Antigravity retains enabled model groups.
- Stale readings retain good values and show the latest error and age. Failed
  rows retain their reason and offer inline recovery. Refresh indicates ongoing
  work and does not accept another button action while busy.
- The window remains layered, topmost, a tool window and non-activating. Shadows
  are CPU-painted; their margin does not participate in hit testing. The saved
  position remains the pill's surface anchor rather than its shadow margin.

## Checks executed

- The new width regression failed before implementation: the inspector was
  520 px wide instead of the selected 386 px. It passes with the new layout.
- `go vet` passed for the application, widget, quota, Win32, raster, marks, core,
  text and process packages.
- Fresh tests passed for the application, widget, quota, raster, marks, core,
  text and process packages.
- `go test ./internal/win -run 'TestOrbit|TestLoop' -count=1` passed. The new real
  window policy test verifies its required extended styles, MA_NOACTIVATE,
  unchanged foreground window, and rejection of a shadow-only point.
- Interaction geometry was tested at 100%, 125%, 150% and 200% DPI, both above
  and below the rail. Separate controls, disabled refresh, compact sizing,
  the panel bridge, deterministic resets and premultiplied alpha pass.
- A saved pill anchor remains unchanged when shadow padding is introduced.
- `go run . --shot .debug-launch/orbit-native` completed. Normal, compact,
  expanded-group, stale/failed, checking, empty, and 150% output were generated.
  Native renderer screenshots were inspected for spacing, clipping, hierarchy,
  marks, numeric alignment, failure states and model-group values.
- `go run . --print` completed with exit 1 in the build sandbox. Codex returned
  usage. Claude, Cursor and Devin reported connection failures; Kiro and Copilot
  reported unreadable CLI responses; Antigravity reached its usage timeout.
  No failed reading was converted into a successful sample value.
- Changed Go files are formatted and `git diff --check` passes.

## Existing full-check failures

These failures were reproduced before the Orbit implementation and remain:

- `go vet ./...` and `go test ./...` cannot compile the unrelated, untracked
  `internal/brand/preview_temp_test.go`: its Pose, Sample, WidgetProfile and
  Render definitions are missing.
- `internal/win/TestFlushPresentsOpaquePixels` fails its actual-screen pixel or
  hit check in this environment. Serializing package execution also reproduced
  it. The separate Orbit policy and window-thread checks pass; this does not
  make the old desktop-pixel test a pass.
- `gofmt -l .` reports the already modified `internal/proc/proc.go` and
  `internal/proc/proc_windows.go`. Those unrelated edits were preserved.

A manual live mouse walkthrough and a move between physical monitors at
different DPI were not performed. The PNGs show the actual native renderer
with frozen illustrative data, rather than a capture of live account values.

## Installed executable

The original workspace executable was backed up under
`.debug-launch/agentquota-before-orbit-*.exe` before replacement. Only processes
whose executable path exactly matched this workspace's `agentquota.exe` were
stopped. No coding-agent processes were targeted.

The final verified binary replaced the workspace executable and was restarted
outside the build sandbox with its normal network access. The installed hash
matches the candidate built after the final checks:

`345995685CFBDFA0127E3E97B4CA1C6F72CFA5EA32172A0D26AB01D7FE2534DC`

The restarted process was PID 58484. It reported the expected executable path
and Responding=True. No public release, installer, or signed build was created.
