# Installer, brand, and motion implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a per-user Windows installer, original B2 logo assets, and coordinated brand and widget animation.

**Architecture:** A small `internal/brand` package owns normalized geometry and deterministic poses. Asset generation, the CPU widget renderer, and packaged installer frames consume that package. The existing Win32 loop schedules widget motion only while the inspector is visible, and Inno Setup owns installation and removal.

**Tech stack:** Existing Go 1.27 module, CPU raster painter, Win32, build-only Inno Setup, and a separate Hyperframes preview project.

**Spec:** [Approved design](../specs/2026-10-15-installer-brand-motion-design.md).

## Global constraints

- Windows only. Keep no cgo, Gio, or GPU dependency in the installed app.
- Preserve the seven readers, credentials, polling deadlines, focus-free window, drag position, and reset-display behavior.
- Brand colors are `#131116`, `#FFFFFF`, and `#8BB8FF`. Keep existing quota status colors.
- Brand timing is 4 seconds, with 1.2 seconds of movement. Widget timing is 6 seconds, with 0.9 seconds of movement.
- Widget mark is 20 logical pixels, at the inspector header's right side with a 16-pixel inset. Preserve the 40-pixel header and closed pill geometry.
- Movement targets 30 frames per second. Holds use a single wake-up timer. Closed or reduced-motion surfaces have no animation timer.
- Install `AgentQuotaSetup.exe` under `%LOCALAPPDATA%\Programs\AgentQuota`, with settings remaining in `%APPDATA%\agent-quota`.
- Desktop and first-install autostart options start unchecked. Launch after installation starts checked. Preserve an existing autostart choice on upgrade.
- No publishing, certificate purchase, credential changes, or support-data deletion in this plan.
- Use GPT-6 Luna at high reasoning for workers and reviewers, as requested. The primary agent owns integration and live checks.

## Review focus

1. Rapid hover, click, reveal, and close events must not leave timers running or restart an already visible cycle. Task 4 exercises these transitions.
2. Windows animation preferences can change during playback or fail to read. Task 4 verifies immediate static fallback and recovery.
3. Smaller icons and fractional DPI can close seams or clip the offset corner. Tasks 1 and 3 inspect actual-size exports and assert bounded rendering.
4. Paths containing spaces or non-ASCII characters must install and launch correctly. Task 6 verifies a real installation in such a path.
5. Repeated setup and uninstall must preserve user settings and unrelated Run values. Task 6 verifies content and registry ownership before and after each operation.

## Execution and file map

Throughput checkpoint: build canonical geometry first; then Tasks 3 and 5 can run independently. Task 6 waits for assets and the resource-enabled application build. Keep each writer's files separate and serialize changes to `main.go`, `internal/win/win.go`, and `DESIGN.md`.

- `internal/brand/geometry.go`, `motion.go`, and `render.go` own the mark, poses, and rendering.
- `cmd/brand-assets/main.go` generates checked-in brand assets and reproducible packaging frame output.
- `assets/brand/` holds SVG, PNG, ICO, lockups, font license, and asset attribution. `dist/` holds ignored executable and frame builds.
- `widget/widget.go` consumes explicit logo pose state. `widget/brand_test.go` verifies rendering invariants.
- `logo_motion.go` and `logo_motion_test.go` own the application's motion lifecycle. `main.go` wires visibility and events.
- `internal/win/motion_windows.go` owns native animation timers and preference reads. Existing callbacks and test callbacks receive preference-change events.
- `brand-motion/` is a separate Hyperframes project for the four-second preview. It consumes generated SVG frames and timing metadata.
- `packaging/agentquota.iss`, `animation.iss`, and `version.json` own setup, artwork playback, and version metadata.
- `scripts/build-installer.ps1` owns asset generation, version resources, Go build, and Inno compilation. `scripts/verify-installer.ps1` records installation verification.
- `README.md`, `THIRD_PARTY_NOTICES.md`, and `DESIGN.md` document usage, packaged notices, and the approved visual system.

### Task 1: Canonical B2 geometry and motion

**Files:** Create `internal/brand/geometry.go`, `motion.go`, `render.go`, and `brand_test.go`.

**Interfaces:**

- `type Point struct { X, Y float64 }`, `type Transform struct { X, Y, Degrees float64 }`, and `type Pose [4]Transform`. The zero pose is locked.
- `type Profile struct { Cycle, Move time.Duration }`. Export `BrandProfile` and `WidgetProfile` with the exact durations above.
- `func Sample(elapsed time.Duration, profile Profile) (Pose, time.Duration)` returns the current pose and delay until the next visible frame or cycle. During a hold, return the remaining hold duration.
- `func Draw(c *raster.Canvas, bounds image.Rectangle, pose Pose, ink color.RGBA)` and `func Render(size int, pose Pose, ink, background color.RGBA) *image.RGBA`.
- `func SVG(size int, pose Pose, ink color.RGBA) string` exports the same four polygons and transforms. Keep geometry private and immutable.

- [ ] Write `TestProfileTiming`, `TestLockedEndpoints`, `TestHoldSchedulesNextCycle`, and `TestRenderingDeterminism`. Assert `BrandProfile == Profile{4*time.Second, 1200*time.Millisecond}` and `WidgetProfile == Profile{6*time.Second, 900*time.Millisecond}`. Assert zero pose at the movement endpoint and cycle boundary, a nonzero pose during movement, and a 5100 ms delay at the widget hold's start. Identical inputs must produce identical image bytes.
- [ ] Run `go test ./internal/brand` and confirm these behavioral assertions fail before implementation.
- [ ] Implement the four B2 polygons from the selected reference, preserving the upper-right locking detail. Use existing `raster.FillPathRect` and explicit point transforms. Limit the widget displacement and rotation to half the brand version's values. Define the movement in one shared sequence, with the locking section settling last.
- [ ] Run `go test ./internal/brand`. Render locked and displaced marks to inspect geometry, clipping, antialiasing, and the motion endpoint. Refine proportions against B2 before exporting consumers.
- [ ] Commit this independently testable brand package.

### Task 2: Production assets and reproducible exports

**Files:** Create `cmd/brand-assets/main.go`, `cmd/brand-assets/main_test.go`, and `assets/brand/`. Update `.gitignore` to ignore `/dist/`, preview caches, and generated packaging scratch.

**Interfaces:** `go run ./cmd/brand-assets --out assets/brand --frames-out dist/brand-frames` exports the static asset family, 36 brand movement frames at 30 fps, one locked frame, and timing metadata. Frame filenames are `frame-000.bmp` through `frame-035.bmp`, plus `locked.bmp`. Export the matching SVG frames for Hyperframes. Make `--out` and `--frames-out` exact output boundaries and never delete other files in either directory.

- [ ] Write `TestICOContainsAllSizes` with expected sizes `[16,20,24,32,40,48,64,128,256]`, `TestPNGHasTransparentBackground`, `TestExportPathsWithSpaces`, and `TestFrameCountAndLockedPose`. Read actual encoded outputs, including ICO directory entries, rather than testing filenames alone.
- [ ] Run `go test ./cmd/brand-assets` and confirm the missing exports fail.
- [ ] Implement deterministic SVG, PNG, BMP, and PNG-backed ICO export using the standard library and existing `x/image/bmp`. Read a verified Source Sans 3 Semibold font file for packaging wordmarks through `x/image/font/opentype`. Keep this font in `assets/brand`, not the widget font registry. Retain its official OFL and source attribution. Produce a separate small-size optical variant only where actual-size inspection requires wider gaps.
- [ ] Run the export command twice and compare hashes. Inspect the mark on light and dark backgrounds at every ICO size and review the wordmark spelling and spacing. Compare the silhouette against supported agent marks and researched references. Record visual checks without claiming trademark clearance.
- [ ] Run `go test ./cmd/brand-assets ./internal/brand`, then commit the exporter and approved static assets. Generated installer frames remain build output.

### Task 3: Pure widget rendering

**Files:** Modify `widget/widget.go`. Create `widget/brand_test.go`.

**Interfaces:** Add `LogoPose brand.Pose` to `widget.View`. `Render(View)` draws the mark at `image.Rect(g.px(g.w-36), g.px(g.panelY+10), g.px(g.w-16), g.px(g.panelY+30))`. No motion clock or timer is read by the renderer.

- [ ] Write `TestBrandDoesNotChangePill`, `TestBrandDoesNotChangeHitBounds`, `TestClosedPanelIgnoresPose`, and `TestBrandAtFractionalDPI`. Compare entire pill image bytes and all existing hit rectangles across locked and moving poses. Exercise scales 1, 1.25, 1.5, and 2. Same view and pose must produce identical pixels.
- [ ] Run `go test ./widget` and confirm the new mark-specific assertions fail.
- [ ] Add the right-aligned header mark in ice blue using Task 1's `Draw`. Preserve title position, layout constants, agent colors, and `Size`, `PillBounds`, `RefreshBounds`, and `ResetRowBounds` behavior.
- [ ] Run `go test ./widget` and `go run . --shot dist/shots`. Inspect open, closed, and empty states. Commit the rendering integration.

### Task 4: Visible-only native widget motion

**Files:** Create `logo_motion.go`, `logo_motion_test.go`, `internal/win/motion_windows.go`, and `internal/win/motion_windows_test.go`. Modify `main.go`, `internal/win/win.go`, and `internal/win/win_test.go`.

**Interfaces:**

- `type logoMotion struct { started time.Time; visible, enabled bool }` with `Set(visible, enabled bool, now time.Time)` and `Sample(now time.Time) (brand.Pose, time.Duration)`. Preserve start time when visibility and enabled state are unchanged. Disabled or closed returns locked pose and zero delay.
- `win.ArmLogoTimer(delay time.Duration) error`, `win.StopLogoTimer()`, and `win.AnimationsEnabled() bool`. Reserve timer ID 102 and map it to `menuLogoTick = 102` in `app.OnMenu`. Kill the native timer before dispatch so each arm is one-shot. Existing poll timer 101 stays independent.
- Add `OnSettingsChanged()` to `win.Callback` and its existing implementers. Forward `WM_SETTINGCHANGE` so the app rechecks the client-area animation preference.

- [ ] Write `TestLogoLifecycle` for open, hold, repeat, close, reopening, and redundant reveal. Write `TestAnimationPreferenceChange` for disabling mid-movement, query failure, and re-enabling while open. Verify exact 6 s cycle, 900 ms movement, hold wake-up, and zero delay when disabled or closed. Assert unchanged visibility does not restart motion.
- [ ] Run `go test . ./internal/win` and confirm the new lifecycle assertions fail.
- [ ] Implement the lifecycle using Task 1's sampler. Centralize application open-state transitions in `setPanelOpen(open bool)` and use it from hover, click, and reveal. Feed the pose into `buildView`. Arm timers only for a visible enabled mark. Timer failure logs a reason and settles the mark without interfering with polling.
- [ ] Implement preference reads with `SystemParametersInfoW(SPI_GETCLIENTAREAANIMATION)` and static fallback on error. Cancel timers on close, quit, and preference disable. Keep native calls on the existing window thread. Prevent late timer messages from restarting closed motion.
- [ ] Run `go test . ./internal/win ./widget`. Exercise the real layered window through hover, pin, reveal, close, drag, DPI change, and Windows animation settings. Verify focus stays with the previous app. Record timer start-stop evidence and inspector screenshots at locked, displaced, and settling poses.
- [ ] Commit native motion integration.

### Task 5: Four-second brand preview

**Files:** Create the separate `brand-motion/` Hyperframes project, its `BRIEF.md`, composition, package scripts, and generated-frame manifest.

**Interfaces:** Consume Task 2's SVG frame exports and timing metadata. A square 1080-pixel composition shows the same B2 silhouette on graphite. The first 1.2 seconds play the movement frames, followed by the locked pose until 4 seconds. Reduced-motion mode displays only the locked frame.

- [ ] Check `npx hyperframes usage --json`, update the `motion-graphics` workflow with `npx hyperframes skills update motion-graphics`, and read its installed instructions before authoring. Follow the core, animation, CLI, and media contracts. Scaffold an empty `brand-motion` directory, then write the confirmed brief. Pin the resolved CLI version in project scripts.
- [ ] Wire deterministic, seekable playback from the exported frames. Do not reconstruct the logo or motion in JavaScript. Keep sound, camera movement, text motion, glow, and flashes absent.
- [ ] Run the pinned project's `check --snapshots` command. Inspect opening, peak displacement, lock completion, hold, and the loop boundary. Confirm playback frame ordering and a clean static reduced-motion state.
- [ ] Open the Studio preview, verify its URL responds, and present it for review. Render the final export only after the user's render approval, as required by Hyperframes. Verify actual output duration, dimensions, absence of audio, and the opening and closing frames.
- [ ] Commit the reproducible preview project and its asset manifest. Keep rendered delivery files in ignored output directories.

### Task 6: Native animated installer and application resources

**Files:** Create `packaging/agentquota.iss`, `animation.iss`, `version.json`, `winres.json`, `scripts/build-installer.ps1`, `scripts/verify-installer.ps1`, `internal/proc/running_windows.go`, and `running_windows_test.go`. Modify `main.go` and add installer-check tests to `main_test.go`.

**Interfaces:**

- `proc.RunningExecutable(path string) (bool, error)` performs a read-only exact executable-path check through Windows process enumeration and process image queries. Normalize absolute Windows paths before comparison. Do not rely on basename or the global single-instance mutex. A process that exits during inspection is not an installation failure; a relevant process whose identity cannot be determined returns an error.
- `agentquota.exe --check-install-target PATH` exits 0 when replacement is clear, 2 when the exact target is running, and 1 on inspection failure. Handle it before any detachment, credential reader, Claude integration, or window creation. No process is terminated.
- `scripts/build-installer.ps1 -Version 0.1.0 -IsccPath PATH` generates frames, embeds resources, builds the app, and compiles `dist/AgentQuotaSetup.exe`. Store the version in `packaging/version.json`; use numeric Windows resource version `0.1.0.0`. Expose optional `-SignToolCommand` as an explicit build input, and propagate signing errors rather than publishing an unsigned success.
- `scripts/verify-installer.ps1 -SetupPath PATH -InstallDir PATH` runs installation scenarios in an explicitly named disposable directory and records assertions in `dist/verification/installer.json`. The verification script must resolve paths before cleanup and preserve any existing startup value and settings it did not create.

- [ ] Write `TestRunningExecutableExactPath`, `TestRunningExecutableExitedProcess`, and `TestInstallerCheckHasNoLaunchSideEffects`. Use a harmless helper process at the target path, a same-name helper at another path, and a path with spaces and non-ASCII characters. Assert clear/running/error exit behavior and absence of a new widget or quota reads from check mode. Run `go test . ./internal/proc` and confirm the new behavior fails before implementation.
- [ ] Implement the read-only process check and early flag. Keep all native enumeration details inside `internal/proc`. Add the user-facing close-and-retry wording in the installer, not the process helper.
- [ ] Install Inno Setup on the build machine if absent, using its official distribution. Record the compiler version in build output. Use build-only `github.com/tc-hib/go-winres@v0.3.3`, pinned to its published release, to generate a root `rsrc_windows_amd64.syso` with the approved ICO and version resources. Keep the manifest's requested execution level `asInvoker` and preserve the existing native DPI handling. This adds a build tool because the Go compiler needs Windows resource input; it adds no imported runtime dependency. Exclude generated `.syso` files from Git and fail compilation if resource generation fails. Build Windows amd64 for the first release. Do not change module dependencies for packaging.
- [ ] Implement `agentquota.iss` with stable AppId `{{DA633FEA-0D52-42E3-9B67-098184910164}`, `PrivilegesRequired=lowest`, `DefaultDirName={localappdata}\Programs\AgentQuota`, `CloseApplications=no`, and `RestartApplications=no`. Include the app, static artwork, notices, and uninstall registration. Create the current-user Start entry and optional unchecked desktop task. Launch after setup by default, except in silent verification runs.
- [ ] Implement autostart selection with an unchecked first-install task. On upgrade, initialize it from the actual current Run entry when that entry targets the installed app, since the app's menu can change it after setup. Do not rely only on Inno's previous task state. Apply the user's setup choice after successful installation. Quote paths correctly. Remove an unchecked or uninstalled Run value only if its parsed executable path still matches this install's app. Leave unrelated or retargeted values untouched.
- [ ] In `PrepareToInstall`, extract a temporary copy of the app payload and run its exact-target check against `{app}\agentquota.exe` with a bounded hidden child. A running target produces a close-and-retry prompt; a process-check failure produces a reason and stops replacement. Repeat the check before writing files. Never use force-close or restart-manager termination. If replacement still races with a new launch, surface the file-in-use failure without deleting the old installation.
- [ ] Implement the welcome-page artwork in `animation.iss`. Preload all 36 BMP movement frames and the locked frame into `TBitmap` objects once. Display them through `TBitmapImage.Bitmap.Assign`. Import native `SetTimer` and `KillTimer` with pointer-sized types and create the callback through `CreateCallback`. Use elapsed time to choose frames so delayed callbacks do not slow the sequence. Stop the frame timer during the hold and arm one wake-up for the next cycle.
- [ ] Start artwork only on its visible welcome page, using `CurPageChanged`. Clear an active flag before killing timers, and check it in every callback because a canceled timer may already have queued a message. On `DeinitializeSetup`, cancel timers before freeing bitmaps and controls. Silent setup and an unreadable or disabled Windows animation preference show the locked mark without a timer. Recheck the preference before subsequent cycles so a setting change settles artwork.
- [ ] Run `scripts/build-installer.ps1 -Version 0.1.0 -IsccPath PATH`. Verify `dist/AgentQuotaSetup.exe` exists, the application PE has the correct ICO and version, and unsigned output is explicitly labeled unsigned. Run `go test . ./internal/proc` after embedding resources.
- [ ] Exercise fresh setup, Start launch, Unicode custom path, both shortcut choices, both autostart choices, repeated setup, app-menu autostart changes followed by upgrade, and uninstall. For upgrade, compare settings and Claude snapshot file hashes before and after. Keep a development copy running during one setup and confirm it does not trigger the installed-target conflict. Keep the installed copy running during another and confirm close-and-retry works without terminating it.
- [ ] Verify uninstall preserves support data and a Run value retargeted to another executable. Inspect native installer animation, page navigation, cancel, reduced motion, and final teardown. Use a clean Windows environment without Go to prove the installed payload has no toolchain dependency. Record any unavailable environment as missing coverage, never a pass.
- [ ] Commit packaging, resource configuration, and exact-target checks after the actual installer checks pass.

### Task 7: Documentation and integrated acceptance

**Files:** Update `DESIGN.md`. Create `README.md` and `THIRD_PARTY_NOTICES.md`. Store ignored verification artifacts under `dist/verification/`.

- [ ] Extend `DESIGN.md` with approved B2 identity, native placement, timing, motion-preference behavior, and asset ownership. Replace its invalid `{typography.body}` component reference with a concrete supported primitive or explicit component font properties. Preserve the existing widget design decisions and Inter typography.
- [ ] Document `AgentQuotaSetup.exe` as the installer and `agentquota.exe` as the application. Cover Start launch, optional desktop and Windows startup settings, upgrade, uninstall data retention, build prerequisites, and unsigned-build status. Explain that generated-brand previews require development tooling while the installed application does not.
- [ ] Assemble the notices for the shipped Go dependencies, donor code, agent artwork, Inter, Source Sans, and build-tool notices required for distributed outputs. Use actual license files and component versions, not inferred licenses.
- [ ] Run `gofmt -l .` with no output, `go vet ./...`, `go test ./...`, and `go run . --shot dist/final-shots`. Run the design-file validator and check its token references. Rebuild the installer from a clean checkout after restoring only documented build tools.
- [ ] Review the whole diff against the approved spec. Re-run only checks affected by review fixes. Verify the installed application's icon, native panel motion, focus behavior, static fallback, and settings preservation against the same build being handed over.
- [ ] Present the final app installer, logo assets, preview URL or approved export, test evidence, and any missing coverage. Do not upload or publish the release without an explicit request. Commit the final documentation and accepted review fixes.

## Planning evidence and execution handoff

The current repo has Go installed and no discovered Inno compiler. `internal/raster` already supports polygon path drawing, `widget.Render(View)` is pure, the inspector header has room for the mark, and the existing native timer dispatch can carry a separate event. Windows client-area animation handling, app-brand assets, PE resources, and installer files do not exist yet.

Inno's official [CreateCallback example](https://jrsoftware.org/ishelp/topic_isxfunc_createcallback.htm) imports `SetTimer`; its [class reference](https://jrsoftware.org/ishelp/topic_scriptclasses.htm) exposes `TBitmap` and `TBitmapImage`. Its [script events](https://jrsoftware.org/ishelp/topic_scriptevents.htm) provide page-change and teardown hooks. Native animation therefore needs a small explicit player rather than a built-in animated-logo control. [go-winres v0.3.3](https://github.com/tc-hib/go-winres/releases/tag/v0.3.3) is the pinned resource build tool.

Self-review mapped identity to Tasks 1 and 2, motion to Tasks 1, 3, 4, and 5, installation to Task 6, and documentation and integrated acceptance to Task 7. The shared `Pose`, `Profile`, and export interfaces match their consumers. Each Review focus item has a named check. No product code or assets changed during planning.

Recommended execution is subagent-driven, with fresh GPT-6 Luna high workers for implementation and fresh reviewers per task. The primary agent integrates shared files and owns live Windows and installer verification. First create an isolated worktree through the using-git-worktrees skill. Read the approved spec and this plan in every worker brief. No implementation begins until the user reviews this plan and selects or confirms the execution method.
