# AGENTS.md — agent-quota

Builds `agentquota.exe`, a floating quota pill for 7 agents. Windows-only Go module. No cgo, Gio, or GPU.
Spec: `docs/specs/2026-10-13-agent-quota-widget-design.md`. Donor: sibling `../Hover-main`. Copy trimmed files from the donor. Never develop in Hover.

## Build with this toolchain

Install Go first, then use these exact deps.

- Run `winget install GoLang.Go`, then reopen the shell so PATH picks it up. Verified with Go 1.27.
- `go.mod` keeps `regexp2`, `x/sys`, and `x/image`. `go mod tidy` drops `typesetting` because nothing uses it yet. Re-add `typesetting` when real text shaping lands.

## Copy code into this layout

All paths below exist, and `go vet ./...` is clean.

- `quota/` holds `Reading{Used *float64; Detail; Reset5h time.Time}` (the 5h-window reset instant the UI reads). It holds the parsers and the readers for claude, kiro, codex, cursor, copilot, devin, and antigravity. It also holds `ByID` and `Present`. `schedule.go` holds `Book` and `Poller` with Stale. Stale keeps the last good reading beside the latest attempt. Hover has no Stale.
- `quota/antigravity.go` reads usage through the `agy` CLI's `/usage` command (agy 1.1.11+), with a five-second version check and a 20-second print-mode timeout. It uses the CLI's own keyring and accepts grouped quota buckets only when the command returns usage rather than a model turn.
- `quota/claude_snapshot.go` receives Claude Code's documented status-line `rate_limits`. Only usage percentages, resets, observation time, and hashes are saved under the app's support folder, scoped by Claude config and session. Identical timer callbacks keep the original observation time. Fresh snapshots take precedence over the existing OAuth reader; credentials remain read-only. A snapshot event refreshes only Claude via `Poller.RefreshOne`.
- `claude_statusline.go` owns `--install-claude-feed` and `--claude-statusline`. Installation preserves the user's existing status-line renderer and other settings. The bridge forwards the original stdin and stdout, with bounded child execution, through Claude Code's Windows shell: Git Bash when installed, otherwise PowerShell. Never persist the full status-line input or an entire Claude settings backup.
- `quota/copilot.go` runs `gh api copilot_internal/user` with a 15 s deadline. The token stays in gh's keyring. The ring shows the max over non-unlimited `quota_snapshots` of `100 - percent_remaining`. All-unlimited shows 0% and "Unlimited".
- `quota/devin.go` parses two values from `%APPDATA%\devin\credentials.toml` by hand, with no dependency. It posts `GetUserStatus` with `Connect-Protocol-Version: 1` and the released CLI metadata (`devin-cli` / `chisel`). It reads the semantic version following `devin` once, ignoring the trailing build hash, and uses it for both version fields. Unknown versions fail before HTTP. Quotas and string reset times come from `userStatus.planStatus`; the plan name is in that object's `planInfo`. HTTP 400 is a request failure, not a reason to sign in again.
- `internal/core` copies `json.go`, `stamp.go`, and `readfile_windows.go` verbatim. `core.go` adds only `AppData`, `Support`, and `Logf`. `Support` points at `%APPDATA%\agent-quota`. `Logf` writes to stderr and `failure.log`. Codex logs and the Cursor DB need the share-mode `Open` and `ReadFile`.
- `internal/proc` trims Hover `agents`. It keeps `Find`, `Hidden`, `Spawn`, and `Home`. `Find` falls back to the Kiro `ProgramFiles` and `LOCALAPPDATA` folder. `Hidden` sets `CREATE_NO_WINDOW`. `Spawn` uses a job object and kills the tree on timeout. Use these deadlines: Kiro 25 s, `gh` 15 s, and HTTP 15 s.
- `internal/raster` and `internal/text` are minimal CPU painters. They provide `Ring`, the rounded rect, and `basicfont`. `internal/marks` holds the ring colours: green below 70, amber from 70, and red from 90. It also holds `Initials` placeholders. Copilot, Devin, and Antigravity asset sources and licenses are listed in `internal/marks/assets/ATTRIBUTION.md`.
- `internal/win` is a layered `WS_EX_LAYERED|TOPMOST|TOOLWINDOW|NOACTIVATE` popup. It paints with `UpdateLayeredWindow`. It answers `WM_MOUSEACTIVATE` with `MA_NOACTIVATE`. It drags with `SetCapture` after 4 px. It never uses `HTCAPTION`. It claims the `Local\AgentQuotaRunning` mutex, and a second launch exits. Read the already-exists signal from `Call`'s errno, not `windows.GetLastError`, which reads nil here. Autostart writes `HKCU\…\Run\AgentQuota`.
- `widget/` is pure. `Render(View)` returns `image.RGBA` with no window, so tests and `--shot` use it directly. `main.go` owns settings at `%APPDATA%\agent-quota\settings.json`. It also owns the `--print` and `--shot DIR` flags. `Reading.Reset5h` feeds the pill's second line and the panel reset row for codex/claude/antigravity from one instant, so the surfaces never disagree; a panel-row click toggles countdown/clock on both, in-memory, default countdown. Window mode re-runs itself detached (`AGENTQUOTA_DETACHED`) so the launching terminal returns at once.

## Verify with these commands

All green 2026-10-13.

- Run `gofmt -l .` and expect no output. Run `go vet ./...`. Run `go test ./...`. The suite holds new `copilot_test`, `devin_test` with `httptest`, `stale_test`, and `widget_test`. It also holds copied `quota_upstream_test`, `schedule_upstream_test`, `num_test`, and `cursor_windows_test` with the real `winsqlite3.dll`. It also holds the core `json`, `stamp`, and `readfile` tests.
- Run `go run . --print`. It reads all seven agents once. It exits 1 when any reader fails. An expired Claude sign-in, a bad Devin `ideVersion`, or an unsupported Antigravity CLI response show as Failed with a reason. That is expected.
- Run `go run . --shot DIR`. It writes `pill.png`, `panel.png`, and `empty.png`, one file per state.

## Follow the rules agents miss

These are the mistakes a new session makes without help.

- Treat each reader as a cheap `Present()` plus a deadline `Read()` on its own goroutine. Treat creds as read-only. Never log creds, and never cache them. Keep one in-flight read per agent. Poll every 30 s. Re-read after 5 min. Refresh forces all readers.
- Keep the window focus-free. Do not add Esc handling or click-outside handling. Save the position on drag release. Use the primary work area so the pill and panel stay clear of the taskbar. Recover unusable saved positions to top-right; preserve valid drag positions. A second launch posts a reveal request to the running window and exits; the existing inspector opens without activation.
- Read the Cursor token through the system `winsqlite3.dll` in `sqlite_windows.go`. Leave the pure-Go `sqlite_reader.go` out on purpose. It carries `//go:build !windows`.
- Copilot, Claude, and Devin publish no API. A break shows as Failed with a reason. It never crashes. The other agents keep working.
- Write comments that explain why. Keep diffs surgical. Give the reason in the message when you add a dependency. Ignore the Gio, office, CI, and release sections of Hover docs.
