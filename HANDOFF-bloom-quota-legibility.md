# Handoff — Bloom quota ring + agent logo legibility

Updated: 2026-11-12. Scope: make the AI-agent brand logo and its quota ring bigger /
more readable in Bloom. CSS edits are written but **not yet verified against a live
render**. A fresh agent should finish verification, fix any visual defect, and stop.

This file is separate from `HANDOFF.md` (the Go native deskband / Seam handoff). Do not
merge them; that one is authoritative for the Windows Go host, this one for the Bloom UI.

## Where the work lives (read first)

- The CLI session was opened in `C:\Users\Bennett\BANTAYOG`, so the shell keeps resetting
  there. **That is not the project.** BANTAYOG was not touched; its dirty tree
  (`.kiro/steering/*`, `AGENTS.md`, `docs/paypal-hackathon/`, `reports/`, `research_notes/`)
  is the user's own pre-existing work.
- The real project is `C:\Users\Bennett\Documents\PROJECT\agent-quota\bloom` (Tauri 2 +
  React 19 + Vite 7). `cd` there for every command. The Go module is the parent
  `agent-quota/`.
- Bloom already carries a large uncommitted WIP branch (MyFinder, Dock removal, Rust
  changes). The legibility change below is a small slice on top of it — keep it surgical.

## User intent

The user showed a macOS desktop screenshot of Bloom's full-width lavender **MyFinder**
bar and said the agent logo and the ring indicator are "very small for the user's eye."
True intent: enlarge the brand mark + quota ring so they read at a glance. This is a
bounded refinement of existing code, not a redesign. An earlier palette-direction study
was superseded by this size fix — do not revive it.

## What is already changed (uncommitted, 3 CSS files)

All sizes are CSS px. The ring SVG `viewBox="0 0 28 28"` (outer r=12.25, inner r=9.5) in
`src/quota/QuotaPanel.tsx` was **not** changed; CSS scales it. Logo size = ring − 2·inset.

- `bloom/src/myfinder.css` (the screenshot surface; bar is 24px tall minimum):
  - ring + svg 20 → **22**; badge box stays 24.
  - `.qs-mark` inset 5 → **4** (logo 10 → **14**).
  - track + fill stroke-width 2.4 → **2.75**.
  - inner track **and** inner fill now always `display:none` → **single ring** in the bar
    (was: hidden only when `[data-empty]`).
  - `.myfinder-quota-value` 12 → **13**, added `font-weight:600`.
- `bloom/src/quota/QuotaSpace.css` (notch panel; keeps the dual ring):
  - `.qs-ring` / `.qs-ring-svg` 28 → **32**; track + fill stroke 2 → **2.5**.
  - `.qs-mark` inset 7 → **8** (logo 14 → **16**).
  - `.qs-pct` 11.5 → **12.5**; `.qs-cell-top` height 32 → **36**; `.qs-rings` height 72 → **78**.
  - header comment now says sizes run a step larger than the Go painter (inks still mirror).
- `bloom/src/App.css` (notch status row is 36px):
  - `.notch-quota-badge` and its `.qs-ring` 28 → **32**; `.notch-quota-strip button` 28 → **32**.

## Key design decision (why single ring in the bar)

Two concentric tracks mathematically cap the centre logo at ~0.5× the ring diameter
(logo must fit inside inner r=9.5). In a 24px bar the ring maxes at ~22px, so a dual-ring
logo can never exceed ~11px — the user's complaint cannot be fixed while keeping both
tracks there. Decision: **MyFinder bar = one bold ring + 14px logo**; the second quota
window stays in the hover card and the `%` text beside the mark. **The notch keeps the
dual ring** (more room; it is the primary quota UI and is e2e-covered).

Geometry to re-check visually:
- Bar single ring 22px, stroke 2.75: outer inner edge ≈ 8.5px radius; 14px rounded tile
  corner ≈ 8.4px → fits with ~0.1px clearance. Tight; confirm it does not kiss the arc.
- Notch dual ring 32px, inset 8 (logo 16): tile corner ≈ 10.3px vs inner-track inner edge
  ≈ 9.4px → **~0.9px overlap** of the thin inner track at the four diagonals. Likely
  invisible, but if it reads as broken, use inset 9 (logo 14, no overlap) or thin the
  inner stroke.

## Hard constraints / safety (binding, from agent-quota/AGENTS.md)

- **Never launch `bloom.exe`** on this Windows box — it hides the taskbar; closing must go
  through `restore_taskbar_and_exit`. Verify in a browser harness / Playwright instead.
- **Never launch the floating `agentquota.exe` window.** The only quota UI is the Bloom notch.
- Leave the quota helper running: `%LOCALAPPDATA%\com.sehaz.bloom\agentquota.exe --quota-service`.
- Credentials are read-only; never log, cache, or return secrets / PINs / keys.
- Keep diffs surgical; comments explain *why*. Preserve the notch dual-ring and its e2e.
- Do not `git commit` / `push` / `reset --hard` / `clean -f` unless explicitly asked.

## Verification status — NOT done yet

The last e2e run reported "37 failed", but every test failed in ~4ms because the vite dev
server had stopped when the prior turn was cut (Playwright could not connect; its
`webServer.command` is `bun run dev` and **bun is not on PATH**). That is a harness
artifact, **not** a code failure. Nothing has been confirmed against a live render.

Steps to verify (Git Bash, inside `bloom/`):
1. Boot vite directly (bypasses the missing bun):
   `./node_modules/.bin/vite.exe --port 1420 --strictPort` (background). Port is fixed 1420.
2. Run e2e — `playwright.config.ts` has `reuseExistingServer:true`, so it reuses step 1:
   `./node_modules/.bin/playwright.exe test e2e/quota-space.spec.ts e2e/status-widgets.spec.ts --reporter=list`
   Chromium is installed (`%LOCALAPPDATA%\ms-playwright\chromium-1234`).
3. Build / type-check (CSS is processed by vite; TS is unchanged):
   `./node_modules/.bin/tsc.exe --noEmit` and `./node_modules/.bin/vite.exe build`.
4. Unit tests use `bun:test` (`scripts/myfinder.test.ts`, `src/quota/*.test.tsx`). bun is
   absent; these assert pure functions / server markup, **not** ring px, so the CSS-only
   change does not affect them. Install bun only if you need to run them; otherwise note
   they were not run.
5. Visual proof (required): a Playwright script that injects `e2e/tauri-mock.js` via
   `addInitScript`, goes to `:1420`, emits a `quota-snapshot` fixture, opens the quota
   space, and screenshots `.qs`. For the bar, clone the rendered badge DOM into a
   `.myfinder-status.myfinder-integrated` wrapper with the lavender material vars and
   screenshot at the true 24px bar height (plus a 2× zoom crop). The e2e mock forces
   `get_myfinder_state → available:false`, so e2e runs in notch mode and the dual-ring
   badge assertion (`quota-space.spec.ts:698`, expects 2 tracks) stays green.

## Acceptance criteria

- Bar: logo visibly larger (~14px), single bold 22px ring fits the 24px bar with ~1px
  margin, no clipping / overlap; `%` 13px/600 readable on both light and dark material.
- Notch panel: rings 32px, logos 16px, pct 12.5px; ring row + every cell stay inside the
  island at Bloom scales 0.8 / 1 / 1.25 (e2e containment); sliding pad still matches the
  selected mark within 1px; dual-ring badge still renders 2 tracks.
- No e2e regressions; `tsc --noEmit` and `vite build` pass.

## Open decisions to confirm with the user (do not silently expand scope)

- Single ring in the bar drops the second-window arc at a glance (data still in hover card
  + `%`). If the user wants both rings preserved there, the logo cannot grow past ~11px —
  surface that tradeoff rather than reverting silently.
- Whether to also enlarge `.qs-name` (10.5px) / `.qs-basis` (10px) under the rings — left
  unchanged to stay surgical.

## Reference artifacts (do not duplicate; read these)

- `agent-quota/AGENTS.md` — project rules + learned preferences (authoritative).
- `agent-quota/DESIGN.md`, `agent-quota/PRODUCT.md`, `agent-quota/GLOSSARY.md`.
- `agent-quota/docs/superpowers/specs/2026-11-11-bloom-myfinder-fit-design.md` and
  `agent-quota/docs/superpowers/plans/2026-11-11-bloom-myfinder-fit.md` — the MyFinder fit
  spec/plan (bar-height math, `fittedBarHeight` min 24).
- `bloom/src/myfinder.ts` (`fittedBarHeight`, `myfinderMaterial`), `bloom/src/windowRegion.ts`.
- `bloom/src/quota/QuotaPanel.tsx` (Ring + QuotaRingMark markup), `bloom/e2e/quota-space.spec.ts`.
- `agent-quota/HANDOFF.md` — the Go deskband/Seam handoff (different subsystem).
- Superseded scratch (ignore): `%TEMP%\bar-direction-study.html` (palette study, not the deliverable).

## Suggested skills (call via the Skill tool)

- `superpowers:verification-before-completion` — before claiming done; run the checks above
  and confirm output, never assert success without evidence.
- `superpowers:systematic-debugging` — if an e2e test fails for real, find the root cause;
  do not bypass or delete the assertion.
- `impeccable` — load `reference/craft-floor.md` before any further UI edit; use its
  polish/refine guidance for the badge contrast and centering.
- `playwright-cli` (or `chrome-devtools-mcp:chrome-devtools`) — render + screenshot the real
  app for the visual check. The chrome-devtools MCP server has been flaky this session
  (disconnected/reconnected); the local Playwright binary is the reliable path.
- `superpowers:test-driven-development` — only if you add a regression test pinning the new
  ring/logo sizes.

Do **not** use the `run` skill or launch `bloom.exe` / `agentquota.exe` here (taskbar hazard).