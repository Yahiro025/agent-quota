# Bloom in MyFinder — implementation record

User approved the integrated material preview and requested implementation on 2026-11-11. This supersedes the earlier opaque black MyFinder housing; standalone Bloom keeps its theme.

## Ownership and approach

1. Extend the existing native maintenance worker. `myfinder.rs` validates `MyFinderApp` and Dock's process, target-monitor geometry, visibility/cloaking/alpha, and publishes changed transient snapshots. Existing WinEvents wake that worker; two-second reconciliation recovers missed events. MyFinder preferences/windows are read-only.
2. Keep the frontend's status row in one shared element. In MyFinder mode it is a transparent content-width strip at a measured height with a readable floor. Each existing panel remains owned by its existing component and opens beneath the strip. Standalone falls back to its existing composition.
3. Scope inherited material and contrast variables to the notch's presentation, without saving them or changing Settings/overlay themes. Use the existing quota view projections in the flat MyFinder menu; retain all agent states, group details, refresh, reset controls, and agent-mark cycling.
4. Report the painted panel bounds on animation frames. Native clipping and click-through use the same split region; no held old panel in the integrated mode. A narrow bridge below the strip preserves pointer continuity. Pin/active use survives MyFinder hiding; idle reveal/peeks are gated.

## Changed owners

- `bloom/src-tauri/src/myfinder.rs`, registered by `main.rs`, observed from `services.rs`.
- `window_region.rs`: reuse successfully applied native regions for MyFinder hit testing.
- `commands.rs`: account for the fitted strip when sizing the existing media window.
- `bloom/src/myfinder.ts`, `hooks/useMyFinder.ts`, `myfinder.css`: fit, contrast, changed-snapshot subscription, scoped styling.
- `App.tsx`: shared status row and independent below-bar panel geometry. Activity affordances remain in the strip.
- `QuotaPanel.tsx`: optional flat menu presentation using the same source facts and detail controls.

No dependency or lockfile changes. Frozen Bun install restored the existing declared dependencies locally. Unrelated working-tree changes are preserved; the pre-existing diff was saved outside the repository under `.gstack/projects/Yahiro025-agent-quota/designs/myfinder-implementation-20261111/pre-existing.patch`.

## Verification and rollout

Frontend build, focused Bun tests, Rust locked check/tests, and an actual bundled-React interaction harness are required. The harness uses mocked Tauri IPC and virtual request routing; it starts no web server and does not prove operating-system behavior. Evidence is under `C:/Users/Bennett/.gstack/projects/Yahiro025-agent-quota/designs/myfinder-implementation-20261111/`.

The release executable is built at `bloom/src-tauri/target/release/bloom.exe`. The installed executable remains at `%LOCALAPPDATA%/bloom/bloom.exe`. Live desktop automation is unavailable in this session (both native automation paths report the native pipe missing), so real MyFinder auto-hide, adjacent menu clicks, and native visual fit cannot be certified here. Do not replace or terminate the running installation without its existing safe exit path; the headless quota helper must remain running.

For later installation, quit Bloom normally through its existing `restore_taskbar_and_exit` path, preserve the previous installed executable for rollback, replace only the Bloom executable, and relaunch it. Never launch the floating quota window. No automatic deployment or installation is claimed by this record.

## Completed proof

- Frontend production build/typecheck passed.
- 108 Bun tests passed across the geometry and quota suites, including material contrast checks and preservation of unknown/stale readings.
- Rust locked tests: 94 library + 174 binary tests passed; one pre-existing ignored test remains. `cargo check --locked` and `cargo clippy --locked --all-targets` passed. Clippy reports pre-existing style/dead-code warnings in existing modules.
- 30 checks passed against the actual React production bundle with mocked native IPC: single strip, all ten rows, mark cycling without opening, below-bar panel, pointer crossing, adjacent-bar region exclusions, waiting/stale facts, pin/hide/reveal, no hidden held-input region, existing panel navigation, 150% Bloom scale, dark material, fullscreen precedence, and standalone fallback. No runtime exceptions.
- Static design checker reported no findings on the new fit/material/helper/style files. Existing unrelated UI was not claimed as globally audited.
- Read-only per-monitor-aware native probe confirmed MyFinder at physical `(0,0)-(1920,30)`. Visible material sample points were owned by MyFinder and returned `(138,111,180)`, `(166,157,220)`, `(143,136,190)`, `(139,140,196)`. MyFinder's ordinary window DC returned white, so the implementation reads only verified visible screen material points. It captures no image.
- Real desktop clicks and a live auto-hide/reveal transition are unverified because native automation is unavailable. Mocked native events do not establish that the installed MyFinder uses those signals.

No settings or installed executable was replaced. The existing quota-service process remains running. Earlier approval-gate text in the specification is historical and superseded by the explicit implementation request.
