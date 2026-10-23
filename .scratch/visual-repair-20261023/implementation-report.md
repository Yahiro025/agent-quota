# Quota-space visual repair — implementation report

Scope: `bloom/src/App.tsx`, `bloom/src/quota/QuotaSpace.css`, `bloom/e2e/quota-space.spec.ts` only. Unrelated dirty work (`bloom/src-tauri/Cargo.toml`, Go tree) untouched. No dock/taskbar/credential/install/autostart changes.

## Audit findings → fixes

- **F1/F2 ring-row overflow + inter-cell bleed (root cause).** Island was a fixed 386px while seven cells need ~450px. The earlier attempt measured `(scrollWidth+24)/scale` — two real errors: (a) `zoom` scales client rects but NOT scroll extents, so scrollWidth was already in the island's unzoomed CSS px and `/scale` undersized at scale≠1 (RED at 1.25: `.qs` right 896.25 > pill right 881.75); (b) `.qs` `width:100%` + `min-width:386` made scrollWidth report ≥386 regardless of need, and `flex:1 0 0` cells bleed into each other *inside* `.qs` where scrollWidth can't see it (RED at scale 1: cell-top 510.33px past its cell edge).
  - Fix: `.qs { width: max-content; min-width: 386px }` — the element itself is the need (intrinsic sizing breaks the feedback; a changed ring row resizes `.qs` and the ResizeObserver re-measures with no reset bookkeeping, so the old `setQuotaSpaceWidth(0)` effect was deleted).
  - `.qs-rings` → `display:grid; grid-auto-flow:column; grid-auto-columns:1fr`: equal columns matching the standalone's even split; `.qs` max-content = n×widest cell so the row's real need reaches scrollWidth.
  - `setQuotaSpaceWidth(target.scrollWidth + 24)` (no `/scale`); island = `max(386, need+24)` — `.qs` sits inside the pill's 12px side padding, all at the same unzoomed boundary.
- **F3 selected vs hover.** Preserved: native CDP shows sel `rgb(28,28,32)` + inset ring/shadow vs hov `rgb(16,16,16)` + none — distinct.
- **F4 footer clearance.** `.qs padding-bottom:8px` preserved: native footerClearance 8.8px.
- **F5 typography.** `.qs-caption` 10px (native confirms). `.qs-name` left at 10.5px: matches the standalone widget's name line; changing it is a design decision outside geometry scope — ruled out-of-scope, not a defect.
- **F6 scroll.** `.quota-viewport` `max-height:100vh; overflow-y:auto` preserved; long-detail e2e asserts scroll + footer reachable. Native: scrollH 215 = clientH 215 (fits, no scroll at rest); groups expand → island 252→312.
- **F7 shrink/unmount.** Leaving quota unmounts `.qs` and calls `set_quota_panel_height 0`; native AFTER-SWITCH: island 312→300, `.qs` gone.
- **Ruled out.** `.qs-mark` svg+img co-exist by design (audit flagged optional/unverified). Kiro lowercase basis labels are real reader output, not a defect. Dark quota inks match the standalone spec.

## Commands & logs

- RED (before fix): `bun run e2e -- -g "seven-agent"` → `e2e-red2.log` — 2 failed (cell-top overflow; scale-1.25 island overflow), 6 passed.
- GREEN: same command → `e2e-green6.log` — 8/8 pass at scales 0.8/1/1.25.
- Full spec: `bun run e2e` → `e2e-green7.log` — 15 pass, 2 beforeEach-startup flakes (`div.bloom` visibility on first compile); retry `e2e-green8.log` — both pass. **17/17 total.**
- Typecheck: `bun x tsc` — clean.
- Unit suite (58 tests) was green pre-change at 18:44 SGT (`unit-green.log`); untouched by this diff, not re-run per instructions.
- Build: `tauri build --debug --no-bundle` via `tauri-build.ps1` (bun/cargo/node on PATH) → `tauri-build2.log`; output `bloom/src-tauri/target/debug/bloom.exe`, built ~00:43 SGT, run pid 27640.
- Native: `launch-verify.ps1` (stops only this checkout's bloom, starts with `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222`) + `verify-repair.js` via CDP → `native-verify4.log`: `qsInPill:true`, all 7 cells `inPill:true` (cell pitch ~61.5px, pctRight inside cells), island 487×252, `.qs` 462.9 inside padding, REFRESH-HIT true, groups +shrink verified. `native-verify3.log` was a stale 13-byte "no main page" run — superseded by verify4.
- Screenshots (physical DPI, `SetThreadDpiAwarenessContext(-4)`, `shot-dpi.ps1`): `native-quota.png`, `native-groups.png` — native app, not mocked. e2e/mocked captures listed in `mock-*.png`.

## Limits & deviations

- The one Impeccable detector pass was not executed — no detector runner is defined in this repo's owned scope; flagging for the parent to run if it has the harness.
- Bloom instance left running from this checkout (pid at launch logged in `native-verify4.log` header); standalone install untouched.
- e2e beforeEach shows occasional first-compile visibility flake under a reused dev server; unrelated to geometry (passes on retry).
- Width floor is effectively 410 (`.qs` min 386 + 24 padding); the `Math.max(386,…)` spec floor is kept as the contract's stated minimum.
