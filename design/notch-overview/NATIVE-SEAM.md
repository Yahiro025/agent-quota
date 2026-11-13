# Native Seam direction contract

The user selected Seam and then selected **Pure matte black** from the two material variants. The authoritative visual is `seam-material-variants/02-pure-matte-black.png`; the prior glass studies are superseded for implementation.

## Quality bar

Operate mode. A compact, quiet seven-agent quota overview at the top centre of the Windows work area, with a full-width upper attachment, concave reverse-rounded shoulders, straight sides and rounded lower corners. Reading surfaces are opaque #000000; selected lift is flat #101010. Inter, authentic marks and 70%/90% usage thresholds remain. No glass rim, backdrop blur, translucency or GPU dependency.

One continuous surface replaces the detached pill/inspector. The seven stable columns show names, usage and their actual quota basis. Selected detail is inline beneath them. Five-hour resets identify the five-hour window even when the headline percentage belongs to the week. Model groups expand with missing windows represented by a dash; failed/stale/checking/empty states remain usable.

Selected detail preserves all reported quota categories, quantities, plans, and associated resets. The selected-detail area grows to fit complete wrapped rows. Stale readings retain those rows alongside the latest error. The earlier Codex footer fix is recorded in `docs/verification/2026-10-16-codex-weekly-visibility.md`; complete seven-agent detail uses the deeper `widget/detail.go` module. Current source mapping, live captures, and installed-build verification are recorded in `docs/verification/2026-10-16-complete-agent-details.md`.

The notch shape requires attachment to the work area's top edge. Dragging below that edge changes Seam to a rounded floating pill or overview with shadow padding on every side. Returning to the top edge restores the notch. Compact mode remains an independent preference; movement never changes it. A mouse-down drag anchor prevents shape-dependent padding from moving the visible rail under the pointer.

Header pin and close, refresh, selected-agent controls, recovery and reset toggles have independent hit targets. Keep the layered window focus-free, 400ms hover reveal, 300ms leave close, no Escape or click-outside handling, 4px capture drag, persistence and single-instance reveal. The settings migration centres the requested new design once; subsequent valid drag and compact settings are preserved. Compact reveal keeps its centre stable.

## Implementation/evidence

- `widget/seam.go` owns the new pure layout, labels, controls and painting; existing Orbit remains a reference/default View path for existing callers/tests. The app explicitly selects Seam.
- `internal/raster/notch.go` paints the contour with CPU curves and exposes matching containment for transparent-side rejection.
- `seam_settings.go` owns one-time position migration. Readers retain more source detail through `quota.Detail`; credential handling, polling, requests, and CLI deadlines remain unchanged.
- Native rendering sizes use physical DPI. The full seven-agent open surface is 528 logical px wide, with a 512px inset body. Its height grows with the selected wrapped details; exterior shadow is excluded from input. The compact resting tab remains 112px wide.
- `.impeccable/review/seam/` contains pure native Render captures at 100/125/150/200% DPI, compact, checking, empty, stale, failed and expanded groups. `panel-on-gray.png` and `panel-on-light.png` composite the exact 150% native rendering over fixed diagnostic backgrounds to expose the contour and material. They are not screenshots of a live editor.
- `live-window.png` captures the installed native window at 125% display scale with physical-coordinate DPI awareness. The parent inspected all seven columns and the matte-black material. The installed window is top-centred; replacement preserved foreground focus and the installed executable hash matches the tested build. Independent finish review covered the native render packet and source, including compact resting, missing-window and recovery evidence; it returned `ship` with no material fixes.

Full `go vet ./...` and uncached `go test ./... -count=1` now pass. Native screen-paint assertions run on the interactive Windows desktop: `TestFlushPresentsOpaquePixels` passes unchanged there, whereas the restricted tool desktop cannot supply its screen probe. The orphaned brand preview experiment is preserved unchanged in `docs/scratch/brand-preview/preview_temp_test.go.txt`, outside compiled packages; the future brand module was not implemented or stubbed. The proc files were formatted without semantic changes, and `gofmt -l .` is empty. No new dependencies.

`seam_detach_test.go` verifies attached/floating contours, stable drag anchors across shape changes, return to the top edge, hover open/close, second-launch reveal, Close, and saved position/Compact/selection preferences. Floating-corner hit rejection and opaque black material are checked at 100/125/150/200% DPI. `--shot` exports include `compact-floating.png`, `pill-floating.png` and `panel-floating.png`. The installed executable matches the final verified build byte-for-byte.

The earlier follow-up `--print` check succeeded for six readers and timed out for Antigravity. Subsequent systematic debugging identified transient backend 503 failures and added one guarded retry inside the existing watchdog. The latest `--print` check exits 0 with all seven readers successful. Reader evidence and regression coverage are recorded in `docs/verification/2026-10-16-antigravity-service-retry.md`; credentials, polling and UI behavior remain unchanged.

No HTML/CSS detector ran for the native implementation: it cannot assess this CPU-painted Win32 surface. Native screenshots/tests and independent review are the verification evidence. Generated reference/background code is illustrative only and is not implementation source.

## Workflow mapping

This is a precisely specified native refinement inside the established Orbit world. The user pinned Seam, the supplied outline and the matte-black material. Impeccable's narrow-request exception applies to concept-seed; no new identity or product interview is required. Existing product facts are the supplied AGENTS.md instructions and `docs/specs/2026-10-13-agent-quota-widget-design.md`. PRODUCT.md was already absent and was not invented as a procedural side effect.

The native Go implementation is code-led with the selected generated visual used as the critique reference. It does not instantiate Product Design's web/mobile template, browser preview, hero gate or web build-phase state. Those procedural artifacts have no verdict on this Win32 layered renderer. The verification packet explicitly distinguishes native rendering proofs from live screen capture, uses project-native focus/hit/placement tests and includes independent finish review. This mapping does not claim unperformed comp-diff, browser or accessibility verification.
