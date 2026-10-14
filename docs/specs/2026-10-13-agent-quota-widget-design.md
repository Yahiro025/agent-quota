# Agent Quota Widget — design

Date: 2026-10-13. Status: for review.

## Intent

The user wants the one feature of Hover that shows AI coding agents' usage limits,
as a standalone Windows app and nothing more: "a hover for checking the usage limits
of all of my AI coding agents … ONLY that feature, nothing else". The code comes
from this repository (Hover-main).

The result: **Agent Quota**, `agentquota.exe`, a small always-on-top pill the user
drags anywhere. Every enabled agent shows its logo inside a ring that fills with
usage (green below 70%, amber from 70%, red from 90%) and a percentage. Hovering or
clicking opens a panel with each agent's detail line and a refresh.

## Decisions (all approved by the user)

1. Small floating widget — not a notch, not a tray app, not a CLI.
2. Seven agents: **Claude Code, Kiro CLI, Codex, Cursor, GitHub Copilot, Devin,
   Antigravity**. OpenCode is dropped (user has no OpenCode Go plan, and Zen
   balance has no API). Zed, crush, and goose have no readable usage. Antigravity
   usage comes from agy's `/usage` slash command through print mode (agy 1.1.11+);
   its own CLI keyring owns sign-in and quota requests stop if the command starts
   a model turn instead of returning usage.
3. A new standalone Go module (not a binary inside Hover), at
   `C:\Users\Bennett\Documents\PROJECT\agent-quota`, module name `agent-quota`.
4. The window is a plain Win32 layered popup drawn on the CPU by Hover's own
   `raster`/`text` painters — no Gio, no GPU, no cgo. (Three independent designs
   converged on this and the user agreed.) Hover's `internal/platform/win` +
   patched Gio + `internal/ui` extraction was considered and rejected: far more to
   copy for a 200-pixel pill.
5. Interaction: rings always visible; hover (~400 ms) opens the detail panel,
   leaving (~300 ms) closes it; a click pins it open, click again unpins. The
   widget never takes keyboard focus, so there is no Esc or click-outside-to-close.
6. A right-click menu: Refresh, one tick per agent to show/hide it, Start with
   Windows, Quit. No tray icon.
7. Start with Windows via `HKCU\…\CurrentVersion\Run` (`AgentQuota` value),
   reusing Hover's `SystemAutostart` shape.
8. Go must be installed first (`winget install GoLang.Go`); it is not on this
   machine.
9. Clicking a Codex or Claude panel row toggles its reset line between countdown ("3h 12m left") and wall-clock ("resets 18:42") instead of (un)pinning the panel. The choice stays in memory only and defaults to countdown.

## Facts verified on the user's laptop

- `gh api copilot_internal/user` works: plan "individual", `premium_interactions`
  173.4/200 remaining, `quota_reset_date` "2026-11-01", `chat`/`completions`
  unlimited.
- Devin's `GetUserStatus` POST returns HTTP 200 with the existing credential and
  complete released CLI metadata. A real version alone with the old metadata
  returns HTTP 400. `devin version` prints `devin 3000.11.3 (9c803229faa4)`;
  use `3000.11.3`, not the build hash. Endpoint:
  `{api_server_url}/exa.seat_management_pb.SeatManagementService/GetUserStatus`
  (server `https://server.codeium.com`), body
  `metadata` contains the existing `apiKey`, `ideName: "devin-cli"`,
  `ideType: "chisel"`, `ideVersion: <ver>`, `extensionName: "chisel"`,
  `extensionVersion: <ver>`, `locale: "en"`, `os: "windows"`, and empty `userJwt`; header
  `Connect-Protocol-Version: 1`. Key and server are read from
  `%APPDATA%\devin\credentials.toml` (`windsurf_api_key`, `api_server_url`).
  Daily/weekly remaining percentages and string Unix reset times are inside
  `userStatus.planStatus`, with the plan name in its `planInfo.planName`.
- On Windows, Cursor's `state.vscdb` is read through the system `winsqlite3.dll`
  (`sqlite_windows.go`); the pure-Go `sqlite_reader.go` is `//go:build !windows`.
- OpenCode's `auth.json` does not exist (auth lives in `opencode.db` / credential
  store); not needed since OpenCode is out.
- `internal/raster` + `internal/text` depend only on `go-text/typesetting` and
  `golang.org/x/image` — no Gio.

## Architecture

Module `agent-quota` (Go only, no cgo, Windows-only for now):

| Package | Purpose | Origin |
|---|---|---|
| `quota/` | Per-agent `Reading{Used *float64; Detail string}`; pure parsers + I/O readers | Copy `internal/quota`: `quota.go`, `read.go`, `num.go`, `sqlite.go`, `sqlite_windows.go`, `keychain_other.go` (its stub keeps `claudeKeychain()` compiling). Drop `daily.go`, `credits.go`, `sqlite_reader.go`, `keychain_darwin.go`. New files: `copilot.go`, `devin.go`, `antigravity.go` (+their reader tests). `ItemAll`/`ItemShort`/`ByID` gain `copilot`, `devin`, `antigravity`. |
| `quota/` (poller) | `Book`/`Poller`: 5-min reads, 30-s tick, forced refresh, one read per id | `schedule.go`, plus a small `Stale` change below |
| `internal/core` | JSON parser, stamps, share-mode file reads, `Logf`, paths, `Day`, `DotnetDouble` | Trimmed copies: `json.go`, `stamp.go`, `readfile_windows.go`, path/log/number helpers — only what `quota` uses |
| `internal/proc` | `Find`, `Hidden`, `Spawn`, `Home` + process-group kill | Trimmed from `internal/agents` (`agents.go`/`proc.go`/`proc_windows.go`) |
| `internal/raster`, `internal/text` | CPU painting, text layout | Copied; `text/hit.go` dropped if unused |
| `internal/marks` | Logo tiles + usage ring + colours | Rewritten from `ui/marks.go`/`markpaths.go` onto `raster`; assets `kiro.svg`, Inter fonts from `ui/assets`; new marks for Copilot (Octicons, MIT), Devin, and Antigravity (PNG) |
| `internal/win` | Layered `WS_EX_LAYERED|TOPMOST|TOOLWINDOW|NOACTIVATE` popup filled by `UpdateLayeredWindow`; mouse events; manual `SetCapture` drag; `WM_MOUSEACTIVATE`→`MA_NOACTIVATE`; per-monitor DPI via `WM_DPICHANGED`; right-click menu | New, ~300-500 lines, bindings in the style of `platform/win/api.go` |
| `widget/` | Collapsed pill + expanded panel, modelled on `notchview.go`'s `pill()`; `Render(state)→image.RGBA` is pure and windowless for tests and screenshots | New |
| `main.go` | Wiring: settings → poller → widget → window; single-instance mutex `Local\AgentQuotaRunning`; flags `--print`, `--shot DIR` | New |

Data flow: 30-s timer → `Poller.Refresh()` → goroutine per due reader → `Book` →
redraw → `UpdateLayeredWindow`. Adding an agent = reader + `ItemAll`/`ByID` case +
logo; the widget is unchanged.

## Widget behaviour

- Collapsed pill: dark rounded bar, items 12 px apart; per agent: 24 px
  logo-in-ring + percent ("42%"). States below. Width fits visible agents.
- Panel (below the pill, or above if near the screen bottom; shifted left/right so
  it stays fully on screen): one row per agent —
  logo, name, percent, detail line ("Pro · day 62% · week 31% · resets 00:00") or
  the failure reason. Footer: "Checked 2 min ago" + Refresh.
- Drag: press + move > 4 px; shorter = click. Position saved on release to
  `%APPDATA%\agent-quota\settings.json`; clamped onto the primary monitor at
  startup if it is off-screen; first launch is top-right.
- Manual `SetCapture` drag (not caption `HTCAPTION`, which is unproven on a
  `NOACTIVATE` window).
- Dark theme only; per-monitor DPI; sharp at 100%/150%.

## Readers

Each reader: `Present() bool` (cheap) + `Read() Reading` with a deadline, on its
own goroutine; never writes or refreshes another tool's credentials.

| Agent | Present | Source (unchanged unless noted) |
|---|---|---|
| Claude | `~/.claude` exists | Prefer a fresh, matching-config Claude Code status-line quota snapshot; fall back to `.credentials.json` → `api.anthropic.com/api/oauth/usage`. Ring = max(5h, week). The client owns sign-in renewal. |
| Kiro | `kiro-cli` found | `kiro-cli chat --no-interactive /usage`, 25 s, process tree killed on timeout |
| Codex | `~/.codex/sessions` exists | newest `rollout-*.jsonl` rate_limits; disk only |
| Cursor | `state.vscdb` exists | token via `winsqlite3.dll` → `cursor.com/api/usage-summary` |
| Copilot | `gh` found | run `gh api copilot_internal/user` (15 s) — gh's keyring handles the token; we never touch it. Ring = max over non-`unlimited` `quota_snapshots` of (100 − `percent_remaining`). Detail: "Individual · premium 26/200 · resets 1 Nov". All-unlimited → 0%, "Unlimited". |
| Devin | `%APPDATA%\devin\credentials.toml` exists | POST `GetUserStatus` as above; `ideVersion` from `devin version` once at startup. Ring = max(100 − `dailyQuotaRemainingPercent`, 100 − `weeklyQuotaRemainingPercent`). Detail: "Pro · day 62% · week 31% · resets 00:00" |
| Antigravity | `agy` found | Require agy 1.1.11+: check `agy --version` (5 s), then run `agy -p /usage --output-format json --print-timeout 20s` (20 s CLI print timeout). Use enabled quota groups' `remaining_fraction` values to show the most-used bucket. The CLI owns its keyring; if `/usage` starts a model turn, stop further quota checks for this run. |

Two-line TOML parsing for Devin's credentials (two string values; no dependency).
Copilot's, Claude's and Devin's endpoints are internal/undocumented; Antigravity
usage relies on the CLI's slash-command contract. A break shows as Failed with a
reason while other agents keep working.

## Failure states and polling

| State | Cause | Pill | Panel |
|---|---|---|---|
| Hidden | not present / unticked | — | — |
| Waiting | no reading yet | grey empty ring "—" | "Checking…" |
| OK | last read ok | ring + % | detail line |
| Stale | last read failed, earlier one ok | last good %, dimmed | error + "last good N min ago" |
| Failed | every read failed | dimmed ring "—" | error |

Stale is new versus Hover: `Book` keeps the last good reading beside the latest
attempt instead of replacing it.

- Poll every 30 s; read any agent whose reading is ≥ 5 min old; Refresh forces all.
- One in-flight read per agent; per-agent deadlines (Kiro 25 s, `gh` 15 s, HTTP 15 s).
- Sleep/wake and reconnect self-heal via staleness. 429 → Stale, "Rate-limited;
  retrying in 5 min". Ticking an agent on reads it at once.
- Credentials are read-only and never logged/cached. App-owned writes include
  `%APPDATA%\agent-quota\settings.json`, failure reasons, and bounded Claude quota
  snapshots containing only usage/reset data, observation times, and hashes.
  Installing the selected Claude feed updates only Claude's `statusLine` setting
  and saves that prior setting for forwarding; it never backs up the whole config.
- Single instance via named mutex; a second launch exits.

## Testing and verification

Claude feed: `agentquota.exe --install-claude-feed` connects the existing status
line through `--claude-statusline`. It preserves the prior renderer's input and
output. Claude Code emits `rate_limits` after a normal model response; installing
the feed does not start a conversation or obtain a new login. Repeated timer
callbacks do not make the observation newer. Snapshots expire after five minutes
and are isolated by config and session, so another account cannot replace the
widget's reading. A new observation refreshes only Claude's row without focus.

1. Install Go (Step 0).
2. Copy Hover's tests unchanged (imports + renamed user-facing strings only):
   `quota_test.go`, `schedule_test.go`, `num_test.go`, `cursor_windows_test.go`
   (real SQLite via winsqlite3), `core` json/stamp/readfile tests.
3. New: `ParseCopilot`/`ParseDevin` fixtures from real responses, personal details
   scrubbed; edge cases (all-unlimited, missing fields, expired reset, bad JSON);
   readers pointed at `httptest` servers (Devin 200/401/500; Copilot via a fake
   `gh`: signed-out, 404); a Stale test; widget layout/hit tests on `Render`.
4. `agentquota --print`: every reader once, seven real rows on this laptop — the
   main end-to-end check.
5. `agentquota --shot DIR`: PNGs of pill + panel in every state from fake readings.
6. Manual checklist: top-right; always on top; never steals focus while
   hover/click/drag; position survives restart; panel open/pin/Refresh; menu
   hide/show; Start-with-Windows writes/removes the Run value; second launch
   exits; sharp at 100%/150%.
7. `gofmt` + `go vet` clean; `go test ./...` passes.

## Non-goals

Notifications, thresholds, history/credits, light theme, tray icon, hiding over
fullscreen, Linux/macOS ports, Hover's office/agents/voice/Settings. Asset sources
and licenses for Copilot, Devin, and Antigravity are recorded beside the marks.

## Open questions (marked, not blocking)

- Whether `internal/text` compiles without `hit.go` (check during implementation;
  keep `hit.go` if anything references it).
- Exact `gh` error wording for "not logged in" and "no Copilot" (parser tests use
  fixtures; live messages recorded at implementation time).
