# Plan: 5h rolling-window reset on pill and panel

Repo: `C:\Users\Bennett\Documents\PROJECT\agent-quota` — Windows-only Go module (`agentquota.exe`). `AGENTS.md` rules are binding.

## Goal

For every agent with a genuine 5h rolling window — Codex, Claude, Antigravity's `"5h"` buckets — show when that window resets:

- **Pill**: a small second line under the percent, e.g. `2h14m` (countdown) or `15:01` (clock).
- **Panel row detail**: `5h resets in 2h 14m` (countdown) or `5h resets 15:01` (clock).
- Clicking a panel row that has a reset toggles countdown ↔ clock for that agent; the pill mirrors the same toggle. In-memory only (`a.resetClock`), default countdown. This exists today for codex/claude; it is extended to antigravity and to the pill.

## Bugs this fixes (why the current text round-trip is being replaced)

1. Claude (OAuth `ParseClaudeUsage` and snapshot `claudeSnapshotReading`) and Antigravity put the reset of the *most-used* window in `Detail`, which is often the weekly one.
2. Codex prints `resets HH:mm` with no date; `main.go`'s `parseResetClock` treats a post-midnight reset as already passed and drops the countdown.
3. The countdown only redraws when a reading changes (up to 5 min apart); the 30 s poll timer must redraw.

## Global constraints (verbatim, binding)

- Surgical diffs; comments explain *why*; do not add or remove unrelated comments.
- No new dependencies. No cgo, Gio, or GPU.
- Credentials read-only, never logged, never cached.
- `Render` stays pure: no `time.Now`, no I/O. `widget/` carries no window dependency.
- `Detail` strings produced by readers stay byte-identical; `--print` output format unchanged.
- `Reading.Reset5h` semantics: zero `time.Time` = no usable 5h reset; a reset at or before `now` counts as none.
- Reset shows only on `quota.OK` rows (Stale keeps its error line; Failed/Waiting have none). After the reset passes it disappears until the next reading.
- Pill height stays `pillH = 40` and slot percent width stays `PctWidth`; if the reset text cannot fit `PctWidth` legibly, raise `PctWidth` uniformly for all items (never per-item) and report it.
- Focus-free window rules unchanged: no Esc handling, no click-outside handling.

## Out of scope

- Devin (day/week windows), Kiro/Cursor/Copilot (monthly resets): untouched.
- `ClaudeFiveHourReset` (quota.go): leave as is.
- Hover donor tree: never develop there.
- Window mode must not be launched during verification (it detaches and stays running).

## Toolchain

Go is NOT on bash PATH. From Git Bash use `/c/Program\ Files/Go/bin/go.exe` and `/c/Program\ Files/Go/bin/gofmt.exe`, or from PowerShell `& "C:\Program Files\Go\bin\go.exe"`. Verification: `gofmt -l .` (no output), `go vet ./...`, `go test ./...`, `go run . --shot DIR` then visually inspect `pill.png`/`panel.png`. `go run . --print` exits 1 while Claude's sign-in is expired — expected.

## Method

TDD per task: write the failing test first, show RED evidence, implement, show GREEN. Commit per task.

## Task 1: quota — Reading carries the 5h reset instant

- `quota/quota.go`: add `Reset5h time.Time` to `Reading` (zero = no 5h reset; comment why). Convert `newReading` and any positional `Reading{...}` literals to keyed fields.
- `ParseCodexLine`: set `Reset5h` only when the **primary** window's label resolves to `"5h"` (window_minutes ≈300 or missing → fallback label `"5h"`) and its reset is after `now`. Reuse the reset already computed inside the `read` closure — do not add a third parse.
- `ParseClaudeUsage`: set `Reset5h` from `five_hour.resets_at` when after `now`, regardless of which window is `top`.
- `claudeSnapshotReading` (`quota/claude_snapshot.go`): set from `newest.FiveHour.ResetAt` when `FiveHour.Present` and after `now`.
- `antigravityReading` (`quota/antigravity.go`): earliest future `reset` among buckets with `window == "5h"`.
- Keep all `Detail` strings byte-identical — existing upstream tests pass unchanged.
- Tests (in `quota/reset_test.go` or per-file test files, matching existing style):
  - Claude week-heavier (`seven_day` utilization > `five_hour`) still yields the five_hour instant.
  - Codex primary with `window_minutes: 150` → zero `Reset5h`.
  - Codex/Claude expired reset → zero.
  - Antigravity: earliest future `reset_time` among `"5h"` buckets; weekly-bucket resets ignored.
  - Snapshot path: `claudeSnapshotReading` sets `Reset5h` from the five-hour window.

## Task 2: widget contract + main wiring

Depends on Task 1's `Reading.Reset5h`. This task defines the widget contract Task 3 renders.

- `widget/widget.go` `Item`: replace `ResetCountdown`, `ResetClock`, `ShowClock` with `Reset string` (the panel line, already toggled) and `PillReset string` (the pill second line, already toggled). Delete `resetText`; `ResetRowBounds` guards on `it.Reset != ""`; `detail()` appends `it.Reset` exactly as `resetText` was appended. Pill rendering does NOT consume `PillReset` yet — that is Task 3; leave a comment on the field.
- Update `widget/reset_test.go` to the new fields (same coverage: deterministic render, bounds empty-cases, row stride).
- `main.go`:
  - Delete `resetStrings`, `parseResetClock`, `parseHM`, `resetMonth`, `formatCountdown`, and their tests in `main_test.go`.
  - Add pure helper `resetLabels(r time.Time, now time.Time, clock bool) (panel, pill string)`:
    - zero or `!r.After(now)` → `"", ""`.
    - countdown (floor to minutes): panel `"5h resets in 2h 14m"` / `"5h resets in 12m"`; pill `"2h14m"` / `"12m"`. Under one minute → `"0m"` / `"5h resets in 0m"`.
    - clock: panel `"5h resets " + quota.FormatLocal(r, "HH:mm")`, or `"d MMM HH:mm"` when not the same local day as `now` (compare `r.Year()/r.YearDay()` after converting to `time.Local` — `sameLocalDay` is unexported in quota); pill always `quota.FormatLocal(r, "HH:mm")`.
  - `buildView`: for `st == quota.OK` and non-zero `show.Reset5h`, set `it.Reset`, `it.PillReset` from `resetLabels(show.Reset5h, now, a.resetClock[id])`. Strip `Detail`'s trailing `" · resets X"` only when X equals `quota.FormatLocal(Reset5h, "HH:mm")` or `quota.FormatLocal(Reset5h, "d MMM HH:mm")` (same instant as Reset5h); otherwise leave `Detail` alone so a weekly reset stays visible. Never mutate the poller's `Reading`.
  - `OnPointerClick`: drop the codex/claude-only filter; loop all visible agents — `ResetRowBounds` is empty for rows without a reset, so those clicks still fall through to `OnClick`.
  - `OnMenu` case `101` (30 s timer): call `a.redraw()` after `a.poller.Refresh(false)` so countdown and `Checked N min ago` stay current.
- `main_test.go`: rewrite `TestBuildViewStripsResetTail` / `TestBuildViewKeepsDetailWithoutReset` to feed `quota.Reading{Used, Detail, Reset5h}` via `quota.NewPoller` (pattern already exists there); add a table test for `resetLabels` covering: countdown h+m, countdown minutes-only, clock same-day, clock next-day, expired, zero, and the 22:00 → 02:00 post-midnight case.

## Task 3: pill second line + shot fixtures + docs

Depends on Task 2's `Item.PillReset`.

- `widget/widget.go` `Render` pill loop: when `it.PillReset != ""`, draw percent ~7 px higher (≈ `py+5`, `%` at ≈ `py+6`) and `PillReset` beneath (≈ `py+21`) in a new style ≈ `text.Style{Size: 9, Weight: 500}` with `mutedInk`. No reset → exactly today's single centred line. Pill height (40) and `PctWidth` (31) unchanged.
- Fit-check test in `widget/reset_test.go`: `text.Measure("4h59m", resetFont)` and `text.Measure("23:59", resetFont)` ≤ `PctWidth`. If they fail, raise `PctWidth` uniformly and report it.
- `widget` tests: pill with `PillReset` renders different pixels than without; identical Views render identical pixels.
- `main.go` `shot()`: give claude, codex, and antigravity items static `Reset`/`PillReset` values (e.g. `"5h resets in 2h 14m"`/`"2h14m"`, `"5h resets in 38m"`/`"38m"`). Deterministic — no `time.Now` in shot data.
- `AGENTS.md`: update the `widget/` bullet's reset sentence to describe the new behavior (5h reset on pill + panel for codex/claude/antigravity; panel click toggles both; `Reading.Reset5h` is the source). Keep house style: short, imperative, says why.

## Stop conditions (report instead of improvising)

- A `Detail` string would have to change to make this work.
- Reset text cannot fit legibly in the 40 px pill.
- Antigravity buckets don't actually use the window string `"5h"`.
