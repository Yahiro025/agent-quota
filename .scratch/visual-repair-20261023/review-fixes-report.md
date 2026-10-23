# Review-fix pass on f6a9bf0 — report

Scope: only the two Important findings from `final-review.md`. No redesign, no
new audit round, no Rust changes (trace below shows none were needed).

## Finding → fix mapping

**Important 1 — island width unbounded** (`.qs { width: max-content }` + an
island measured as `scrollWidth + 24` could exceed the monitor):

- `QuotaSpace.css`: `.qs` gains `max-width: var(--qs-max-w, calc(100vw - 24px))`.
  Capped grid cells shrink (existing `min-width: 0`) and their name/basis lanes
  ellipsize instead of bleeding offscreen; `.qs-line` still wraps anywhere.
- `App.tsx` `quotaSpaceMeasureRef`: computes `capW = floor(window.innerWidth /
  scale)` and sets `--qs-max-w = capW - 24` (the pill interior) on the viewport
  element so `.qs` inherits it; the island itself is clamped with
  `Math.min(target.scrollWidth + 24, capW)`. `vw`/`vh` inside the `zoom`
  subtree render scaled, so the bound is computed in JS against the unzoomed
  window size and handed to CSS in the island's own units.

**Important 2 — viewport height ignored the 36px status row**
(`.quota-viewport { max-height: 100vh }` put a long panel's footer below the
window's bottom edge):

- `QuotaSpace.css`: `max-height: var(--qs-max-h, calc(100vh - 36px))`.
- `App.tsx`: `--qs-max-h = floor(window.innerHeight / scale) - 36`, refreshed on
  `window` `resize` (the observer alone never refires: the native
  `set_quota_panel_height` resize doesn't resize `.qs`). At scale 1 the
  fallback `calc(100vh - 36px)` is already exact.

## RED → GREEN

- RED: `bun run e2e -- -g "display bounds"` → `e2e-red3.log`
  - FAIL `long strings keep the island inside the display at scale 1.25`
  - FAIL `the scrolled footer stays inside the window at scale 1` (bottom 516 > 480.5)
  - FAIL `the scrolled footer stays inside the window at scale 1.25` (bottom 645 > 480.5)
- GREEN: same command → `e2e-green9.log` — 6/6 pass (scales 0.8/1/1.25).
- Full suite → `e2e-green10.log` — 23/23 pass.
- `bun x tsc --noEmit` — clean.

New tests (`quota-space.spec.ts`, describe `display bounds`) use a 560px-wide
viewport + ~270-char unbroken strings in err/help/detail-label/group-name lanes
and a 640x480 viewport + the 60-row fixture, asserting `pill.width <=
innerWidth`, `.qs` inside pill, all 7 cells inside pill, `vp.bottom <=
innerHeight`, and the scrolled footer bottom `<= innerHeight` — the physical
clip bound a mocked harness can't otherwise show. Name lane font stays 10.5px.

## Native pointer reproduction (real OS input)

`native-sweep.ps1` moves the physical cursor with `SendInput` absolute moves
(DPI context -4), stepping y=0→30 at x=960 in 2px/25ms steps, sampling
`WindowFromPoint`'s root owner per step (`native-sweep1.log`):

- y=0..28: **Dock_64.exe** (MyDockFinder, PID 14888) owns every point — the
  user's top-docked bar physically occludes the notch's pixel region.
- y=30+: ChatGPT.exe (the window behind).

While parked at (960,14) inside the revealed notch (`native-hover-check.ps1` /
`native-windows.log`), `WindowFromPoint` still returns Dock_64.

**Conclusion:** the traced cause is z-order occlusion by the user's dock, not a
Bloom hook defect. `update_main_interaction` (services.rs ~2664-2840) is
correct: the edge dwell reveals the notch, `in_notch_rect`/`in_top_span` keep
it interactive — but `WindowFromPoint` legitimately identifies the dock's own
pixels above the pill. Winning them would require a topmost war with the user's
dock, which is explicitly out of bounds ("avoid growth/war with user dock",
"do not interfere with user others"). The earlier safety-refused click was the
correct outcome — an OS click at (960,22) would have hit the user's dock, not
Bloom. No Rust change made; fullscreen/drag/transparent-input/pin preserved.

## Packaged build + CDP proof

- Build: `tauri build --debug --no-bundle` → `bloom\src-tauri\target\debug\
  bloom.exe` (3 pre-existing warnings, none new). bloom.exe PID 48908,
  CDP :9222.
- `verify-repair.js` → `native-verify5.log`: `qsInPill:true`, all 7 cells
  inPill, footer inside pill with 8.8px clearance, selected-vs-hover distinct
  (`#1c1c20` + inset ring vs `#101010`), REFRESH-HIT true, groups expanded and
  contained, leaving quotas shrank the window 312→300px with the viewport
  unmounted.
- Native screenshot: `native-quota-fix.png` (real screen, 1200x500 top-center).
  `peek` mode and all user settings untouched; no install/autostart/credential
  changes.

## Changed files

- `bloom/src/App.tsx` — display-bounded caps + resize listener in the observer.
- `bloom/src/quota/QuotaSpace.css` — `--qs-max-w`/`--qs-max-h` caps.
- `bloom/e2e/quota-space.spec.ts` — `display bounds` describe (6 tests).
- `.scratch/visual-repair-20261023/` — `native-sweep.ps1`,
  `native-hover-check.ps1`, logs, `native-quota-fix.png`, this report.

## Remaining limits

- Hit-ack inside the native webview can't be proven at the top edge on this
  machine while MyDockFinder's bar owns those pixels; that is environmental,
  not a regression (the sweep is native input, not CDP/DOM).
- At extreme narrowness (<~400px monitors) cell minimums can still exceed the
  cap; unreachable in practice.
- `e2e-red3.log`'s earlier two width tests passed at 0.8/1 because the reload
  raced the fixture emit; the post-fix runs pass deterministically.
