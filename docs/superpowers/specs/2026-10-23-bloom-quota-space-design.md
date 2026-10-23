# Bloom quota space: the full Agent Quota view inside Bloom's notch

Date: 2026-10-23. Branch: `feat/bloom-agent-quota`. Follows
`2026-10-22-bloom-agent-quota-design.md`.

## Intent

The user wants everything the standalone Agent Quota widget shows and does to
live inside Bloom's notch, in the existing Agent Quotas panel ("the quota
space"). The reference is the standalone's open seam view (screenshot from
2026-10-23: "Used" caption, pin, close, ring row of agents with brand marks,
percentages, names and basis labels, a selected-agent detail block, divider,
refresh and "N failed" footer). The notch may grow as much as the quota space
needs. Once the user signs off on the result, the installed standalone widget
is removed because it is redundant.

Success: the quota space is visually and functionally equivalent to the
standalone open view; Bloom's other panels, resting notch, urgent alert,
Overview and dock behaviour are unchanged; the app stays focus-free.

Approach chosen: rebuild the view in React/CSS/SVG inside Bloom (not streamed
Go-rendered images, not a docked standalone window).

## 1. Layout

Replaces the text-list `bloom/src/quota/QuotaPanel.tsx`. Mirrors
`widget/seam.go` top to bottom, using the widget's ink colours and type scale
(`widget/widget.go`: pill/panel ink, white/dim/muted/error/stale ink; number
11.5/600, percent 9/500, title 12.5/500, detail 11/400, caption 10/400).

- Header (24 px): "Used" caption on the left; pin and close buttons on the
  right. Both are visible only in the quota space.
- Ring row (72 px): one cell per present agent in the order claude, kiro,
  codex, cursor, copilot, devin, antigravity. Agents whose reader is not
  present are omitted, as in the standalone. Each cell: ring with the brand
  mark centred, percentage beside it, agent name below, basis label below that.
  - Basis label follows `seamBasis`: `Checking…` (waiting), `Unavailable`
    (failed), `Stale`, `5h` (when a 5h window exists), `Unlimited`, `Models`
    (groups), the highest window's name (`Week`/`Day`/label), then per agent
    `Credits` (kiro), `Plan` (cursor), `Premium` (copilot), else `Usage`.
  - Ring bands: green below 70, amber from 70, red from 90. No percentage:
    grey ring and `—`.
  - The selected cell has the raised tile shown in the screenshot; hovered
    cells highlight.
- Selected detail block: title `<Name> · <Plan>` then one line per window,
  e.g. `Day · 56% used · resets in 6h 18m`. Also carries the standalone's
  model groups (collapsible, "N model groups"), stale note with last good
  reading, observation time, and for failures the error and recovery hint.
- Divider, then footer (36 px): refresh button (disabled with a spinner while
  any agent is busy) and `N failed` when any reader failed.
- Width at least 386 px. The notch window grows to fit the quota space's
  content height while it is shown and returns to its normal size on other
  panels. Bloom's scale setting applies.

Unchanged: resting notch (quota hidden until urgent), urgent alert at 90%+,
Overview, other panels and panel arrows.

## 2. Data and protocol

- Go: each snapshot reading gains `help` (recovery hint). `recoveryHint`
  moves from `main.go` into `quota/` and both the standalone and service use
  it. No credential ever enters the pipe; existing fields are unchanged.
- Rust (`bloom/src-tauri/src/quota_helper.rs`), both registered in
  `main.rs`:
  - `refresh_quota(id: Option<String>)` writes
    `{"v":<version>,"op":"refresh","id":"all"|<agent>}` to the helper's stdin.
    No-op when the helper is not running.
  - `set_quota_panel_height(px)` resizes the main window to fit the quota
    space, clamped to the monitor height, keeping the top-centre pin from
    `e6d6b22`. Leaving the quota space restores the normal notch height.
- Frontend: `quotaView.ts` gains pure ports of the Go logic: `basis()`,
  `detailLines()`, `failedCount()`, `defaultSelection()`. Resets are formatted
  as countdown or clock from `reset5h` and detail `resetAt` on the existing
  ticker. Busy = any reading with `busy: true`.
- Brand marks: copy `internal/marks/assets` SVG/PNG with `ATTRIBUTION.md` and
  licence files into `bloom/src/quota/marks/`. Claude, Codex and Cursor are
  ported from `internal/marks/markpaths.go` vector paths to inline SVG.
  Missing marks fall back to initials.

## 3. Controls and focus

- Pin: keeps the notch open on the quota space when the pointer leaves.
  In memory only; switching panels unpins.
- Close: collapses the notch, like pointer leave.
- Refresh: `refresh_quota("all")`; disabled while busy. A failed agent's
  recovery hint refreshes that agent only.
- Ring cell click selects it. Selection is in memory; default is the first
  urgent or failed agent, else the first agent.
- Reset click toggles countdown/clock for all resets (in memory, default
  countdown). "N model groups" toggles group expansion.
- Focus-free: no activation, no Esc, no click-outside. Alert deferral
  (typing, dragging, pairing, connecting) is unchanged.
- Clicking the urgent alert opens the quota space with the urgent agent
  selected.

## 4. Testing

- Go: moved `recoveryHint` tests; protocol test that `help` is serialized and
  no credential fields appear; `gofmt -l .`, `go vet ./...`, `go test ./...`.
- Bun: `basis`, `detailLines`, `failedCount`, `defaultSelection`, reset
  formatting, with cases mirrored from `widget/seam_test.go` and
  `widget/reset_test.go`.
- Rust: request-line construction for `refresh_quota`; height clamping;
  `cargo check --locked`, `cargo test --locked`; `bun run build`.
- Playwright on Vite with mocked Tauri, using a fixture matching the
  screenshot (Devin 56% Day, Claude unavailable, Cursor 71% Plan, Codex 10%
  5h, Antigravity 0% 5h): header, ring row, detail, divider, "1 failed";
  select, reset toggle, refresh, groups; screenshot compared to the
  reference.
- Desktop: rebuild debug, relaunch, verify the notch grows in the quota space
  and shrinks on other panels.
- Final, only after the user signs off: uninstall the standalone widget and
  remove `HKCU\Software\Microsoft\Windows\CurrentVersion\Run\AgentQuota`.
