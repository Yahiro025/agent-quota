# Visual audit — Bloom quota space (feat/bloom-agent-quota @ 819ddc5)

Date 2026-10-23. Read-only audit; no product files changed.

## Evidence sources

- **LIVE** — running `bloom/src-tauri/target/debug/bloom.exe` (PID 26288, helper `agentquota.exe --quota-service` PID 30360 alive). `enum-windows.ps1` + `shot.ps1` desktop captures: `live-01-rest.png`, `live-02-click-pill.png`, `live-03-hover.png`, `live-04-click2.png`. CDP on :9222 was **not** available (app launched without `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`; per instructions I did not relaunch it).
- **MOCKED (labelled)** — Vite dev server + `e2e/tauri-mock.js` + 7-agent fixture (all seven readers present; covers failure, stale, groups, dual-window plan). DOM metrics + element/viewport captures: `mock-7agents.png`, `mock-7agents-viewport.png`, `mock-groups.png`, `mock-stale.png`, `mock-failed.png`, `mock-empty.png`. The shipped e2e suite only exercises a **5-agent** fixture, so it cannot see the defects below.
- **SOURCE** — `bloom/src/quota/QuotaPanel.tsx`, `QuotaSpace.css`, `quotaView.ts`, `App.tsx` lines noted per defect.

The user's desktop was actively in use during live captures; pointer exercise was kept minimal.

---

## Defects

### 1. [HIGH] Ring row overflows the island with 7 agents — content bleeds ~32 px past both pill edges

Repro (mocked): emit a snapshot containing all seven agents, open the quota space.
Measured: `div.bloom` = **386 px** wide (x 47–433); `.qs` = **450 px** wide (x 14.9–465). The quota space is ~64 px wider than the island, centred, so ~32 px of content hangs past each edge. `mock-7agents-viewport.png` shows "Claude" clipped at the left pill edge, "Antigravity" clipped at the right, and header/footer text rendered outside the dark pill surface.

Root cause: `App.tsx:2953` — `if (bloomMode === "quotas") return 386;` pins the island width at the spec's *minimum*; nothing widens it for content. `.qs` (`QuotaSpace.css:16-23`, `min-width: 386px`) plus `.qs-rings` (`:69-75`, `padding: 0 16px`) need ≈450 px for 7 cells (7 × ≈60 px content + 32 px padding). The 5-agent e2e fixture needs only ~332 px, so the suite can't catch this.

Expected (spec §1: "Width at least 386 px"): the island grows to fit the ring row (e.g., return `max(386, measuredContentWidth)` alongside the existing height path), or cells are designed to fit 7 agents inside 386 px (smaller ring/pct or horizontal scroll). Growing the island is the spec-faithful fix.

### 2. [MEDIUM] Cell-top contents wider than the cell box at 7-agent density

Repro: same fixture. Measured `.qs-cell-top` widths 57.0–57.5 px vs cell inner box ≈55.75 px (59.75 cell − 4 padding): kiro, copilot, devin rows bleed ~1 px into each neighbour. Ring (28 px) + gap (3) + "90%" (~26 px) ≈ 57–61 px doesn't fit the ~55 px cell at 7-across.

Root cause: `QuotaSpace.css:97-103` `.qs-cell-top` fixed `gap:3px; height:32px` with no overflow guard; cells `flex:1 1 0; min-width:0` (`:77-90`) can't shrink the ring (fixed 28 px, `:105-110`).

Expected: if the island stays 386 px, ring+pct must fit ~55 px (e.g., ring 24 px, pct 10.5 px) or stack pct under the ring. Subsumed if defect 1 is fixed by widening.

### 3. [MEDIUM] Selected cell is visually identical to a hovered cell

`QuotaSpace.css:92-95` — `.qs-cell:hover, .qs-cell.selected { background: #101010; }` shares one rule, so the "selected raised tile" the spec §1 requires ("the raised tile shown in the screenshot") is indistinguishable from hover; on a 7-cell row the user can't tell which agent is selected without reading the detail block.

Expected: distinct selected treatment (raised tile per the standalone's seam view — lighter fill and/or border/inset), hover kept subtler.

### 4. [LOW] Footer sits flush on the island's bottom edge — zero breathing room

Measured: `.qs-footer` bottom == `div.bloom` bottom exactly (251 px). The island height equals the measured content edge, so the 36 px footer's text/button visually rest on the pill's lower rounded corner.

Root cause: `App.tsx:1991-1999` (`quotaSpaceMeasureRef`) + `QuotaPanel.tsx:111-120` measure `.qs` bottom edge exactly; `.qs` has no bottom padding after the footer (`QuotaSpace.css:308-314`, `height:36px` fills to the edge).

Expected: add ~6–8 px bottom padding inside `.qs` (or to the measured extent) so the footer isn't jammed on the pill edge; keeps the footer-inside-island invariant the e2e test asserts.

### 5. [LOW] "Used" caption is 11 px; spec type scale says caption 10/400

`QuotaSpace.css:36-38` `.qs-caption { font-size:11px; font-weight:400 }` vs spec §1 "caption 10/400". Basis labels correctly use 10/400 (`:170`). One-line fix; verify against `widget/widget.go` caption size before changing — if the standalone really renders 10, match it.

### 6. [LOW] Agent name size 10.5 px is outside the spec's type scale

`QuotaSpace.css:159-161` `.qs-name { font-size:10.5px; font-weight:500 }`. Spec §1 scale: number 11.5/600, percent 9/500, title 12.5/500, detail 11/400, caption 10/400 — no 10.5/500. Likely should be 11/500 or 10/500 per the standalone's name row. Verify against `widget/seam.go`/`paint.go` name metrics.

### 7. [LOW, unverified] Each `.qs-mark` emits two drawable elements

Measured 14 matches for `.qs-mark svg, .qs-mark img` across 7 cells — every mark renders both an `svg` and an `img` node. Possibly intentional (icon + overlay) or a double-draw/fallback left in. Check `src/quota/marks.*` (file is `marks.ts` or a folder; `marks.tsx` does not exist) and remove one layer if redundant.

---

## Verified correct (spec-conformant, don't touch)

- Pin unpins on panel switch: `App.tsx:2218`. Urgent alert opens quota space with urgent agent pre-selected: `App.tsx:4364-4365`; island alert sizing `3013-3019`.
- Header 24 px / rings 72 px / footer 36 px sections match spec §1.
- Footer-inside-island fix (819ddc5) verified in mocked run: `footerInPill=true`.
- Default selection = first urgent/failed else first; reset countdown↔clock toggle shared across resets; refresh disabled+spinner while busy; per-agent recovery-hint refresh; groups collapsible with `N model groups`; stale shows last-good + error; `—`/grey ring with no pct; band colours green<70/amber≥70/red≥90 — all confirmed in code and mocked DOM.
- Resting notch hides quota (live capture `live-01-rest.png`: clock-only pill top-centre; standalone widget untouched top-right).
- `quota-viewport` scroll cap (`max-height:100vh`) leaves `.qs` unconstrained for true-height reporting.

## Unverified — needs live CDP or human check

- **Click pass-through (potential HIGH).** One synthetic click at (960,20) reached the app behind the notch (a browser window opened). Likely a synthetic-input artifact: `set_ignore_cursor_events` re-enable on hover may not have processed before the click. But if the resting pill's hit region really doesn't capture clicks, the panel can't open on real clicks either. Re-run via CDP (`set_notch_hovered` + real click) before treating as a defect.
- **Window is 1536×420 at rest** (enum). If cursor-event ignoring ever fails, the top 420 px band eats clicks. Unproven; the same CDP check covers it.
- **Transition shrink** (quota→other panel restores height) and **DPI scaling** — couldn't drive live; `set_quota_panel_height(0)` on unmount (`QuotaPanel.tsx:126`) covers shrink in code.
- **Empty state** rendered in mock (`mock-empty.png`) but my metrics call crashed on a null ring selector — cosmetic script bug, not a product issue.
- **Theme variants** — not exercised; quota space uses fixed dark inks matching the standalone, spec doesn't require theming.
- **`basis()` raw window labels** — Kiro fixture showed lowercase "credits" because my fixture used a lowercase label; spec passes labels through verbatim (`windowName` only title-cases week/day, `quotaView.ts:112-134`). If real Kiro readings carry lowercase labels, the caption will show them — verify against a live Kiro snapshot, not a defect yet.

## Single-batch fix brief

1. Widen the quotas island to fit `.qs` (`App.tsx:2953` + measurement path), or redesign cells to fit 386 px; fixes defects 1–2 together.
2. Split `.qs-cell.selected` from `:hover` styling (`QuotaSpace.css:92`).
3. Add bottom padding below `.qs-footer` (`QuotaSpace.css` footer block or `.qs` padding).
4. Normalize `.qs-caption` to spec 10/400 and `.qs-name` to a spec scale value.
5. Extend the e2e suite with a 7-agent fixture asserting `qs.right <= bloom.right` (the overflow regression test the current suite lacks).
6. Optional: dedupe `.qs-mark` inner elements if confirmed redundant.
