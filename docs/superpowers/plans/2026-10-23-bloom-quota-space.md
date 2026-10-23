# Bloom Quota Space Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Bloom's text-list Agent Quotas panel with a faithful React copy of the standalone Agent Quota open view, with every control working, then retire the standalone widget.

**Architecture:** Go stays the data source and gains one snapshot field (`help`). Rust gains two commands: refresh over the helper's stdin, and fitting the notch window to the quota space's height. The view is pure TypeScript logic (`quotaView.ts`) plus React components under `bloom/src/quota/`, rendered in the existing Agent Quotas panel slot in `App.tsx`.

**Tech Stack:** Go 1.27, Tauri v2 / Rust (`windows` 0.61), React 19 + TypeScript, Bun tests, Vite, Playwright.

**Spec:** `docs/superpowers/specs/2026-10-23-bloom-quota-space-design.md`

## Global Constraints

- Branch `feat/bloom-agent-quota`. Never develop in `../Hover-main`.
- Focus-free: no activation, no Esc, no click-outside handling.
- Never log, cache, or serialize credentials.
- The notch stays pinned to the monitor top centre (commit `e6d6b22`). Never reintroduce a notch AppBar reservation.
- Ring bands: green below 70, amber from 70, red from 90 (`band()` in `protocol.ts`).
- Agent order: claude, kiro, codex, cursor, copilot, devin, antigravity. Absent agents are omitted.
- Pin, selection, groups-open and countdown/clock state are in memory only.
- Conventional commits. Comments explain why only.
- Quota space minimum width 386 px. Header 24, ring row 72, footer 36 px (before Bloom scale).

## Review Focus

1. A snapshot with zero agents shows the empty state, not a blank or crashing panel.
2. Every agent failed: the ring row still renders, the footer reads `N failed`, and the detail block shows the selected agent's error and hint.
3. The helper is down or restarting when Refresh is clicked: nothing throws and the button recovers when the next snapshot arrives.
4. Expanded model groups or long failure text taller than the monitor: the height is clamped and the content scrolls.
5. A `reset5h` time already in the past is shown as `now`, not as a negative countdown.

---

### Task 1: Go sends the recovery hint

**Files:**
- Create: `quota/recovery.go` with `func RecoveryHint(id, reason string) string` moved verbatim from `main.go:701`
- Modify: `main.go` (call `quota.RecoveryHint`, delete the local copy)
- Modify: `quota/service_protocol.go` (reading gains `Help string \`json:"help,omitempty"\``, filled from `RecoveryHint(id, err)` when the state is Failed or Stale)
- Test: `quota/recovery_test.go`, `quota/service_protocol_test.go`

**Interfaces:**
- Produces: the snapshot reading JSON field `help` (string, omitted when empty).

- [x] Move any existing `recoveryHint` cases from `main_test.go` into `recovery_test.go`. If there are none, add `TestRecoveryHintDevinSignIn`, asserting the exact string `main.go` returns today for `("devin", "Sign-in expired.")`.
- [x] Add `TestSnapshotCarriesHelpNotSecrets`. It encodes a Failed devin reading and asserts the line contains `"help":` and contains none of `token`, `apiKey`, `password` or `credential`.
- [x] Run `go test ./quota/...` and expect FAIL.
- [x] Implement it, then run `gofmt -l .` (no output), `go vet ./...` and `go test ./...`, and expect PASS.
- [x] Commit with the message `feat(quota): send recovery hints to the service client`.

### Task 2: Rust refresh command and quota-space height

**Files:**
- Modify: `bloom/src-tauri/src/quota_helper.rs`
- Modify: `bloom/src-tauri/src/commands.rs`, `bloom/src-tauri/src/main.rs` (`generate_handler!`)

**Interfaces:**
- Produces:
  - `pub fn refresh_line(id: Option<&str>) -> String`, which returns `{"v":<protocol version>,"op":"refresh","id":"all"|<id>}\n`.
  - Tauri `refresh_quota(id: Option<String>) -> Result<(), String>`.
  - Tauri `set_quota_panel_height(app, height: u32) -> Result<(), String>`. Pass 0 to restore the normal notch height.
  - `pub fn clamp_height(requested: u32, monitor: u32) -> u32`, which returns `min(max(requested, NORMAL), monitor)`.

- [x] Write the tests:
  - `refresh_line(None)` equals the exact `all` JSON plus `\n`.
  - `refresh_line(Some("devin"))` carries `"id":"devin"`.
  - `clamp_height(2000, 1080) == 1080` and `clamp_height(0, 1080) == NORMAL`.
- [x] Run `cargo test --locked quota` and expect FAIL.
- [x] Implement it:
  - After writing HELLO, `pump` moves `child.stdin` into a `static HELPER_STDIN: Mutex<Option<ChildStdin>>`. This keeps the pipe open, so the existing "do not drop stdin" invariant holds. `note_exit` clears it.
  - `refresh_quota` writes `refresh_line` to it, or returns `Ok(())` when it is `None`.
  - `set_quota_panel_height` resizes the main frame through the existing notch sizing path, then calls `apply_main_appbar_mode(&app, false)` last so the top-centre pin wins.
- [x] Run `cargo test --locked`, `cargo check --locked` and `cargo clippy --locked --all-targets`, and expect PASS.
- [x] Commit with the message `feat(bloom): refresh quotas and fit the notch to the quota space`.

### Task 3: Pure view logic

**Files:**
- Modify: `bloom/src/quota/protocol.ts` (`QuotaAgent.help?: string`)
- Modify: `bloom/src/quota/quotaView.ts`
- Test: `bloom/src/quota/quotaView.test.ts`

**Interfaces:**
- Produces:
  - `basis(a: QuotaAgent): string`
  - `detailLines(a: QuotaAgent, now: Date, clock: boolean): string[]`
  - `failedCount(as: QuotaAgent[]): number`
  - `anyBusy(as: QuotaAgent[]): boolean`
  - `defaultSelection(as: QuotaAgent[]): string | undefined`
  - `formatReset(at: string, now: Date, clock: boolean): string`

- [x] Write the tests. Each case is mirrored from `widget/seam.go` `seamBasis`/`seamDetail` and `widget/reset_test.go`:
  - `basis`:
    - waiting gives `Checking…`, failed gives `Unavailable`, stale gives `Stale`.
    - Windows `[{label:"5h"}]` give `5h`. Summary `Unlimited` gives `Unlimited`. Groups give `Models`.
    - Windows `[{week,28},{day,56}]` give `Day`.
    - With no windows: kiro gives `Credits`, cursor gives `Plan`, copilot gives `Premium`, and any other agent gives `Usage`.
  - `detailLines`: the devin fixture with the photo's values gives `["Day · 56% used · resets in 6h 18m", "Week · 28% used · resets in 2d 6h 18m"]`.
  - `formatReset`:
    - A time in the past gives `now`.
    - `clock: true` gives local `h:mm AM/PM`, using the same format as the Go `reset_test` clock cases.
  - `failedCount`: one failed agent among five gives 1.
  - `defaultSelection`: returns the first urgent (90% or more) or failed agent, else the first agent, else `undefined` for `[]`.
- [x] Run `bun test src/quota` and expect FAIL.
- [x] Implement it, then run `bun test src/quota` and expect PASS.
- [x] Commit with the message `feat(bloom): port the quota seam view logic`.

### Task 4: Brand marks

**Files:**
- Create: `bloom/src/quota/marks/` containing the copied `kiro.svg`, `copilot-24.svg`, `devin.svg`, `antigravity.png`, `ATTRIBUTION.md` and the three licence files from `internal/marks/assets/`.
- Create: `bloom/src/quota/marks/index.tsx`
- Test: `bloom/src/quota/marks/marks.test.ts`

**Interfaces:**
- Produces:
  - `AgentMark({ id }: { id: string }): JSX.Element`
  - `agentName(id: string): string`, which returns `Claude`, `Kiro`, `Codex`, `Cursor`, `Copilot`, `Devin` or `Antigravity`.

- [x] Test that `agentName` gives the seven exact names and that an unknown id falls back to initials (`initials("foo") === "F"`, matching `marks.Initials`).
- [x] Implement it. Claude, Codex and Cursor become inline SVG paths ported from `internal/marks/markpaths.go`, and the other agents import their asset files.
- [x] Run `bun test src/quota` and `bun run build`, and expect PASS.
- [x] Commit with the message `feat(bloom): add agent brand marks with attribution`.

### Task 5: The quota space component

**Files:**
- Rewrite: `bloom/src/quota/QuotaPanel.tsx`
- Create: `bloom/src/quota/QuotaSpace.css`
- Modify: `bloom/src/App.tsx:3713` (props) and the panel pointer-leave handling (pin)

**Interfaces:**
- Consumes:
  - From Task 3: every function it produces.
  - From Task 4: `AgentMark` and `agentName`.
  - From Task 2: `invoke("refresh_quota", { id })` and `invoke("set_quota_panel_height", { height })`.
- Produces: `QuotaPanel({ readings, selected, onSelect, pinned, onPin, onClose }: QuotaPanelProps)`.

- [x] Implement the layout from spec section 1:
  - Header: the `Used` caption on the left, and pin and close buttons on the right.
  - Ring row: SVG ring, mark, percentage, name and basis for each agent. The selected tile and hover highlight use the widget ink colours from `widget/widget.go`.
  - Detail block: the `<Name> · <Plan>` title, then `detailLines`. Then the groups toggle (`N model groups`), the stale note with `lastGoodAt`, the observation time, and the error. The hint is a button that calls `refresh_quota(id)`.
  - Divider, then the footer: a refresh button, disabled with a spinner while `anyBusy` is true, and `N failed`.
  - Empty state when there are no readings.
- [x] Make every control a `<button tabIndex={-1}>` with `onMouseDown={e => e.preventDefault()}`, so nothing takes focus. Add no key handlers.
- [x] Measure the content with a `ResizeObserver`. On each change, call `set_quota_panel_height(scaledHeight)` while the panel is mounted, and `set_quota_panel_height(0)` on unmount.
- [x] In `App.tsx`:
  - Keep `selected` and `pinned` state.
  - While pinned and on the quota panel, skip the pointer-leave collapse. Switching panels unpins. `onClose` collapses the notch.
  - Clicking the urgent alert sets `selected` to that agent before opening the quota panel.
- [x] Run `bun run build` and `bun test`, and expect PASS.
- [x] Commit with the message `feat(bloom): render the full Agent Quota view in the quota space`.

### Task 6: Playwright QA against the reference

**Files:**
- Create: `bloom/e2e/quota-space.spec.ts`, plus a fixture with the photo's values (devin 56% Day plus 28% Week on a Pro plan, claude failed, cursor 71% Plan, codex 10% 5h, antigravity 0% 5h)

- [x] Run Vite. Mock `@tauri-apps/api` `invoke` and `listen` so they emit the fixture as `quota-snapshot`. Assert the following:
  - The texts `Used`, `Devin · Pro`, `Day · 56% used`, `Unavailable` and `1 failed` appear.
  - Clicking the Codex cell changes the title.
  - Clicking a reset swaps to the clock format.
  - Refresh calls `refresh_quota` with `{ id: null }`.
- [x] Save a screenshot and compare it by eye with `C:\Users\Bennett\.t3\userdata\attachments\7065c746-4c11-42b0-9764-7bbaa630c07f-fcdd6bd7-34b7-40df-8c03-2b3ffd2fc8ab.png`. Note any deviations in the task report.
- [x] Commit with the message `test(bloom): e2e the quota space`.

### Task 7: Desktop verification and standalone removal

- [ ] Run `go test ./...`, `cargo test --locked` and `bun run build`. Then build with `tauri build --debug --no-bundle`, copy the helper next to the debug exe, and relaunch Bloom.
- [ ] Confirm on the desktop:
  - The notch grows in the quota space and shrinks back on other panels.
  - It stays at the top centre above MyDockFinder.
  - Pin holds the notch open, and close collapses it.
- [ ] **Only after the user signs off:** stop PID-matched `AppData\Local\Programs\AgentQuota\agentquota.exe`, run its uninstaller or delete that folder (confirm first), and remove `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AgentQuota`.
