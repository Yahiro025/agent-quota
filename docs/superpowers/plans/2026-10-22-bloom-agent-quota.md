# Bloom and Agent Quota Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship one Bloom-derived notch that shows Agent Quota through a headless Go helper, without Bloom's dock or taskbar replacement.

**Architecture:** The existing Go module gains `--quota-service` and speaks versioned JSON lines on stdin/stdout. Bloom stops hiding the taskbar and stops owning a dock window. Its notch keeps the physical top of the primary monitor, hides for fullscreen, and adds Agent Quotas plus Overview panels driven by one visibility controller.

**Tech Stack:** Go 1.27 module (`quota/`, `main.go`), Bloom 3.10.0 Tauri v2 (Rust + React 19), Bun tests for the TypeScript protocol client.

**Spec:** `docs/superpowers/specs/2026-10-22-bloom-agent-quota-design.md`

## Global Constraints

- Helper flag is `--quota-service`. It creates no window, does not detach, and does not acquire `Local\AgentQuotaRunning`.
- Protocol version is `1`. Agent order is `claude`, `kiro`, `codex`, `cursor`, `copilot`, `devin`, `antigravity`.
- States are `waiting`, `ok`, `stale`, `failed`. Times are RFC3339Nano UTC and omitted when zero.
- Credentials never appear on the pipe or in logs added by this work.
- Poll interval stays 30 seconds. Kiro and Antigravity keep their existing longer gap. One in-flight read per agent.
- Urgent expansion is usage `>= 90` or timer completion. Usage `>= 70` and `< 90` does not expand the notch.
- Passive interaction does not focus the window. A clicked text input may.
- Do not hide the Windows taskbar, register an AppBar, or start the taskbar watchdog.
- Do not delete or disable the native Agent Quota window mode.
- Preserve Bloom source and asset notices. Do not rename product `bloom` or identifier `com.sehaz.bloom`.
- Focus assist opens `ms-settings:quiethours` and does not pretend to be on.
- Stage only the files the task lists. The worktree already contains unrelated edits.

## Review Focus

- A helper crash or a refused spawn leaves the last snapshot visible and leaves music, timer, and shelf usable. Tested in Task 7.
- An alert that arrives during typing, dragging, pairing, or an unfinished connection does not change the open panel. Tested in Task 11.
- `stale` keeps the last good `used` and puts the new failure in `err`. Tested in Task 2.
- A second Bloom process must not register an AppBar or hide the taskbar even when a crash marker exists. Tested in Task 4.
- Fullscreen detection must not treat MyDockFinder (`WS_EX_TOOLWINDOW`) or a titled maximized window as fullscreen. Tested in Task 6.

---

### Task 1: Quota service protocol codec

**Files:**
- Create: `quota/service_protocol.go`
- Test: `quota/service_protocol_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `const ServiceVersion = 1`
  - `type ServiceRequest struct { V int \`json:"v"\`; Op string \`json:"op"\`; ID string \`json:"id,omitempty"\` }`
  - `type ServiceWindow struct { Label string \`json:"label"\`; Used float64 \`json:"used"\` }`
  - `type ServiceGroup struct { Name string \`json:"name"\`; Windows []ServiceWindow \`json:"windows"\` }`
  - `type ServiceDetail struct { Label, Value, ResetText string; ResetAt time.Time }` with JSON `label`, `value`, `resetText,omitempty`, `resetAt,omitempty` as RFC3339Nano
  - `type ServiceAgent struct` with JSON fields from the spec Protocol section (`id`, `state`, `busy`, `used`, `detail`, `err`, `plan`, `summary`, `resetLabel`, `reset5h`, `observedAt`, `lastGoodAt`, `windows`, `groups`, `details`)
  - `type ServiceEvent struct { V int; Op, Message string; Agents []string; Readings []ServiceAgent }`
  - `func DecodeRequest(line []byte) (ServiceRequest, error)`
  - `func EncodeEvent(e ServiceEvent) ([]byte, error)` compact JSON, no trailing newline

- [ ] **Step 1: Write the failing test**

```go
func TestDecodeRequestAcceptsHelloRefreshAndShutdown(t *testing.T) {
    hello, err := DecodeRequest([]byte(`{"v":1,"op":"hello"}`))
    if err != nil || hello.Op != "hello" || hello.V != 1 {
        t.Fatalf("hello = %+v, %v", hello, err)
    }
    one, err := DecodeRequest([]byte(`{"v":1,"op":"refresh","id":"claude"}`))
    if err != nil || one.ID != "claude" {
        t.Fatalf("refresh = %+v, %v", one, err)
    }
    if _, err := DecodeRequest([]byte(`{"v":2,"op":"hello"}`)); err == nil {
        t.Fatal("version 2 accepted")
    }
    if _, err := DecodeRequest([]byte(`{"v":1,"op":"refresh"}`)); err == nil {
        t.Fatal("refresh without id accepted")
    }
    if _, err := DecodeRequest([]byte(`{"v":1,"op":"refresh","id":"nope"}`)); err == nil {
        t.Fatal("unknown agent accepted")
    }
}

func TestEncodeSnapshotOmitsZeroTimesAndKeepsZeroUsed(t *testing.T) {
    used := 0.0
    ev := ServiceEvent{V: 1, Op: "snapshot", Readings: []ServiceAgent{{
        ID: "claude", State: "ok", Busy: false, Used: &used,
        Reset5h: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
    }}}
    raw, err := EncodeEvent(ev)
    if err != nil {
        t.Fatal(err)
    }
    text := string(raw)
    if !strings.Contains(text, `"used":0`) || strings.Contains(text, "observedAt") {
        t.Fatalf("encoding = %s", text)
    }
    if strings.Contains(text, "\n") {
        t.Fatal("encoder added a newline")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./quota/ -count=1 -run "TestDecodeRequestAcceptsHelloRefreshAndShutdown|TestEncodeSnapshotOmitsZeroTimesAndKeepsZeroUsed"`
Expected: FAIL, `DecodeRequest` undefined

- [ ] **Step 3: Implement the codec in `quota/service_protocol.go`**

`DecodeRequest` accepts `hello` and `shutdown` with no id, and `refresh` only when id is `all` or one of `ItemAll`. Any other `v` or `op` returns an error. `EncodeEvent` uses `time.Time` fields with a custom marshal so a zero time is omitted and a non-zero time is `Format(time.RFC3339Nano)` in UTC. `Used` is a pointer so `0` is kept and nil is omitted.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./quota/ -count=1 -run "TestDecodeRequestAcceptsHelloRefreshAndShutdown|TestEncodeSnapshotOmitsZeroTimesAndKeepsZeroUsed"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add quota/service_protocol.go quota/service_protocol_test.go
git commit -m "feat: add the quota service protocol codec"
```

### Task 2: Service session over a fake poller

**Files:**
- Create: `quota/service.go`
- Test: `quota/service_test.go`

**Interfaces:**
- Consumes: Task 1 codec. `quota.NewPoller`, `quota.Poller.Refresh`, `quota.Poller.RefreshOne`, `quota.Poller.Display`, `quota.ItemAll`
- Produces:
  - `type Service struct`
  - `func NewService(p *Poller) *Service`
  - `func (s *Service) Handle(line string) (events []ServiceEvent, done bool)`
  - `func (s *Service) Snapshot() ServiceEvent`
  - `func Serve(r io.Reader, w io.Writer, p *Poller) error` reads lines, writes each event plus `\n`, returns nil after `shutdown`

- [ ] **Step 1: Write the failing test**

Cover four behaviors in `quota/service_test.go`:

- `TestHelloRepliesReadyThenWaitingSnapshot` — `Handle("{\"v\":1,\"op\":\"hello\"}")` returns `ready` with the seven ids in `ItemAll` order, then a snapshot of seven `waiting` readings, `done == false`.
- `TestRefreshOneLeavesOtherAgentsWaiting` — poller read func records the id and returns `newReading(12, "ok")` for `claude` only. After `Handle` of refresh `claude` and waiting until the poller's changed callback fires, `Snapshot` has claude `state=ok`, `used=12`, and kiro `state=waiting`.
- `TestStaleKeepsLastGoodUsed` — first read returns `newReading(40, "ok")`; second read returns `Fail("timeout")`. After both complete, claude `state` is `stale`, `used` is 40, `err` is `timeout`.
- `TestShutdownSaysByeAndServeReturns` — `Serve` on a pipe whose stdin is `{"v":1,"op":"shutdown"}\n` writes a line `{"v":1,"op":"bye"}` and returns nil. A line `{"v":1,"op":"nope"}` writes `op=error` and does not end `Serve`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./quota/ -count=1 -run "TestHelloRepliesReadyThenWaitingSnapshot|TestRefreshOneLeavesOtherAgentsWaiting|TestStaleKeepsLastGoodUsed|TestShutdownSaysByeAndServeReturns"`
Expected: FAIL, `NewService` undefined

- [ ] **Step 3: Implement `NewService`, `Handle`, `Snapshot`, and `Serve`**

`hello` emits ready and the current snapshot, then `Refresh(true)`. `refresh` `all` calls `Refresh(true)`. `refresh` of one id calls `RefreshOne`. `shutdown` emits bye and sets done. Blank lines emit nothing. Decode errors emit `error` and done stays false. `Snapshot` reads `Display` for every `ItemAll` id. Map `quota.Waiting/OK/Stale/Failed` to the spec strings. Copy `Used` only when the displayed reading has one. `err` is `StateOf`'s error string. `lastGoodAt` comes from `LastGoodAt` when that time is non-zero. The changed callback is stored so tests can wait; `Serve` writes a new snapshot when it fires. `Serve` does not start its own goroutine beyond what `Poller` already starts.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./quota/ -count=1 -run "TestHelloRepliesReadyThenWaitingSnapshot|TestRefreshOneLeavesOtherAgentsWaiting|TestStaleKeepsLastGoodUsed|TestShutdownSaysByeAndServeReturns"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add quota/service.go quota/service_test.go
git commit -m "feat: speak quota snapshots on a service session"
```

### Task 3: Headless `--quota-service` entry

**Files:**
- Modify: `main.go` (flag parse and the early return only)
- Test: `main_test.go`

**Interfaces:**
- Consumes: `quota.Serve`, `quota.SystemPoller`
- Produces: `func runQuotaService(r io.Reader, w io.Writer) error` which builds `SystemPoller(func(string) bool { return true }, func() {})` and calls `Serve`. The 30-second poll is a `time.Ticker` of `quota.Tick` that calls `Refresh(false)` until `Serve` returns. `--quota-service` calls `runQuotaService(os.Stdin, os.Stdout)` and returns before detach and `win.HoldSingle`.

- [ ] **Step 1: Write the failing test**

`TestQuotaServiceFlagSkipsTheWindow` in `main_test.go` sets `os.Args` to `agentquota --quota-service` is not required. Instead call a new `func quotaServiceSelected(args []string) bool` and assert true only for `--quota-service`. Assert `runQuotaService` is not invoked by the test (it would read real agents). A second test `TestRunQuotaServiceUsesServe` is not added here if it would hit the network. Document in the test name that selection is pure.

```go
func TestQuotaServiceFlagIsSelectedWithoutAWindow(t *testing.T) {
    if !quotaServiceSelected([]string{"agentquota", "--quota-service"}) {
        t.Fatal("flag not selected")
    }
    if quotaServiceSelected([]string{"agentquota", "--print"}) {
        t.Fatal("print selected the service")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -count=1 -run TestQuotaServiceFlagIsSelectedWithoutAWindow`
Expected: FAIL, `quotaServiceSelected` undefined

- [ ] **Step 3: Implement the flag and `runQuotaService`**

Parse `--quota-service` with the existing `flag` set. When it is set, call `runQuotaService(os.Stdin, os.Stdout)` and return. That call sits before the `AGENTQUOTA_DETACHED` branch and before `win.HoldSingle`. `runQuotaService` starts a goroutine that ticks `quota.Tick` and calls `Refresh(false)` on the poller passed into `Serve`. Do not log reading bodies.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -count=1 -run TestQuotaServiceFlagIsSelectedWithoutAWindow`
Expected: PASS

Then: `go test ./quota/ . -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: add a headless quota service mode"
```

### Task 4: Notch startup never hides the taskbar

**Files:**
- Create: `bloom/src-tauri/src/dock_lifecycle.rs`
- Modify: `bloom/src-tauri/src/main.rs` (module declaration and the setup closure's dock/taskbar calls)
- Test: `bloom/src-tauri/src/dock_lifecycle.rs` (`#[cfg(test)]`)

**Interfaces:**
- Consumes: nothing from the Go tasks
- Produces: `pub struct StartupPlan { pub hide_taskbar: bool, pub register_dock_appbar: bool, pub start_watchdog: bool, pub open_dock_window: bool }` and `pub fn notch_startup_plan() -> StartupPlan` with every field `false`.

- [ ] **Step 1: Write the failing test**

```rust
#[test]
fn notch_startup_does_not_touch_the_taskbar_or_dock() {
    let plan = notch_startup_plan();
    assert!(!plan.hide_taskbar);
    assert!(!plan.register_dock_appbar);
    assert!(!plan.start_watchdog);
    assert!(!plan.open_dock_window);
}
```

- [ ] **Step 2: Run test to verify it fails**

Run from `bloom/src-tauri`: `cargo test --locked notch_startup_does_not_touch_the_taskbar_or_dock`
Expected: FAIL, cannot find `notch_startup_plan`

- [ ] **Step 3: Implement the plan and gate setup**

`setup` in `main.rs` calls `notch_startup_plan()` and skips `init_taskbar_marker`, `start_taskbar_watchdog`, `restore_taskbar_after_crash`, `set_taskbar_visibility(false, ...)`, `register_dock_appbar`, and dock-window `unwrap` when the matching field is false. A crash marker is removed if present and the taskbar is restored only when the marker says a previous process hid it (`restore_taskbar_after_crash`), which is recovery, not hiding. Do not call any function that hides the taskbar. Leave dock command handlers in place but make `init_dock` and `register_dock_appbar` return immediately when `notch_startup_plan().open_dock_window` is false.

- [ ] **Step 4: Run test to verify it passes**

Run from `bloom/src-tauri`: `cargo test --locked notch_startup_does_not_touch_the_taskbar_or_dock`
Expected: PASS

Run: `cargo check --locked`
Expected: success

- [ ] **Step 5: Commit**

```bash
git add bloom/src-tauri/src/dock_lifecycle.rs bloom/src-tauri/src/main.rs
git commit -m "fix: stop Bloom startup from hiding the taskbar"
```

### Task 5: Dock window is not required to start

**Files:**
- Modify: `bloom/src-tauri/tauri.conf.json` (remove the `dock` window entry)
- Modify: `bloom/src-tauri/src/main.rs` (every `get_webview_window("dock").unwrap()`)
- Modify: `bloom/src-tauri/capabilities/default.json` if it names the dock window
- Test: `bloom/src-tauri/src/dock_lifecycle.rs`

**Interfaces:**
- Consumes: `notch_startup_plan` from Task 4
- Produces: `pub fn dock_window_configured() -> bool` returning `false`

- [ ] **Step 1: Write the failing test** `dock_window_is_not_configured` asserting `dock_window_configured() == false`.

- [ ] **Step 2: Run it** from `bloom/src-tauri`: `cargo test --locked dock_window_is_not_configured` Expected: FAIL

- [ ] **Step 3: Remove the dock window from `tauri.conf.json`.** Replace dock `unwrap()` with `if let Some(dock_win) = ...`. `dock_window_configured` returns false. Do not delete `src/Dock.tsx`; nothing creates that window.

- [ ] **Step 4: `cargo test --locked dock_window_is_not_configured` and `cargo check --locked`** Expected: PASS and success

- [ ] **Step 5: Commit** the files this task modified, message `fix: start the notch without a dock window`

### Task 6: Fullscreen suppression

**Files:**
- Create: `bloom/src-tauri/src/fullscreen.rs`
- Modify: `bloom/src-tauri/src/main.rs` or `services.rs` only where the notch visibility loop already runs
- Test: `bloom/src-tauri/src/fullscreen.rs`

**Interfaces:**
- Consumes: nothing from quota tasks
- Produces: `pub struct ForegroundProbe { pub covers_primary: bool, pub tool_window: bool, pub no_activate: bool, pub has_caption: bool, pub class_name: String, pub notification_state: u32 }` and `pub fn notch_hidden_for(probe: &ForegroundProbe) -> bool`

`notch_hidden_for` is true when `notification_state` is 3 or 4 and `covers_primary` is true. It is true when `covers_primary` and not `tool_window` and not `no_activate` and not `has_caption`, and `class_name` is not `Progman`, `WorkerW`, `Shell_TrayWnd`, `Shell_SecondaryTrayWnd`, `XamlExplorerHostIslandWindow`, `Windows.UI.Core.CoreWindow`, `MultitaskingViewFrame`, or `ForegroundStaging`. It is false for a tool window even when `notification_state` is 2. It is false when `has_caption` is true and `notification_state` is not 3 or 4.

- [ ] **Step 1: Write tests** for those four cases, named `exclusive_fullscreen_hides`, `presentation_hides`, `toolwindow_does_not_hide`, `titled_maximized_does_not_hide`.

- [ ] **Step 2: Run** `cargo test --locked -- fullscreen::` Expected: FAIL

- [ ] **Step 3: Implement `notch_hidden_for`.** The existing notch visibility loop calls it and hides the `main` window without activating another window when it returns true, and shows `main` again when it returns false. Do not hide for the dock. Do not call `SetForegroundWindow`.

- [ ] **Step 4: Re-run the same cargo test and `cargo check --locked`.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: hide the notch over primary fullscreen`

### Task 7: Spawn the helper and keep the last snapshot

**Files:**
- Create: `bloom/src-tauri/src/quota_helper.rs`
- Modify: `bloom/src-tauri/src/main.rs` (register the command and start the helper in setup)
- Test: `bloom/src-tauri/src/quota_helper.rs`

**Interfaces:**
- Consumes: the JSON schema from the spec Protocol section
- Produces:
  - `pub fn helper_command(exe: &Path) -> Command` argv is `exe --quota-service`
  - `pub fn apply_line(state: &mut HelperState, line: &str) -> Apply` where `Apply` is `Ready`, `Snapshot`, `Bye`, `Error`, or `Ignore`
  - `pub struct HelperState { pub readings: Vec<serde_json::Value>, pub unavailable: bool }`
  - On an `error` or a line that is not JSON, `unavailable` stays false if a snapshot already exists. On process exit, `unavailable` becomes true only when `readings` is empty; otherwise readings stay and `unavailable` stays false.

- [ ] **Step 1: Tests** `helper_argv_is_quota_service`, `snapshot_replaces_readings`, `exit_keeps_last_snapshot`, `exit_without_snapshot_is_unavailable`.

- [ ] **Step 2: Run** `cargo test --locked -- quota_helper::` Expected: FAIL

- [ ] **Step 3: Implement the parser and a supervisor** that spawns `agentquota.exe` beside the current executable, writes `{"v":1,"op":"hello"}\n`, and emits Tauri event `quota-snapshot` with the readings JSON whenever a snapshot applies. Restart after exit. Do not read stderr into the protocol.

- [ ] **Step 4: Re-run the cargo test and `cargo check --locked`.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: supervise the quota helper from Bloom`

### Task 8: TypeScript protocol client

**Files:**
- Create: `bloom/src/quota/protocol.ts`
- Test: `bloom/src/quota/protocol.test.ts`

**Interfaces:**
- Consumes: snapshot JSON from the spec
- Produces: `export type QuotaState = "waiting" | "ok" | "stale" | "failed"` and `export function parseSnapshot(line: string): QuotaAgent[] | null`. `parseSnapshot` returns null for non-snapshot lines. For `stale`, `used` is kept and `err` is set. `band(used: number): "green" | "amber" | "red"` returns `green` below 70, `amber` from 70 and below 90, `red` from 90. `export function urgent(agent: QuotaAgent): boolean` is true only when `used` is present and `>= 90`.

- [ ] **Step 1: Write `protocol.test.ts`** using `bun:test` for a snapshot line, a stale agent, `band` boundaries 69, 70, 89, 90, and `urgent` false at 89 and true at 90.

- [ ] **Step 2: Run** from `bloom`: `bun test src/quota/protocol.test.ts` Expected: FAIL

- [ ] **Step 3: Implement `protocol.ts`.**

- [ ] **Step 4: Re-run the same bun test.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: parse quota snapshots in the notch`

### Task 9: Agent Quotas panel

**Files:**
- Create: `bloom/src/quota/QuotaPanel.tsx`
- Create: `bloom/src/quota/QuotaPanel.test.tsx` only if the component can be rendered without a DOM library. If it cannot, keep the behavior test in `protocol.test.ts` as `viewModel(readings, openPanel)` instead of a component test, and make `QuotaPanel` a thin render of that view model.
- Modify: `bloom/src/App.tsx` to add the panel id `quotas` beside existing panel ids, opened the same way other panels open.

**Interfaces:**
- Consumes: `parseSnapshot`, `band` from Task 8. Tauri event `quota-snapshot`.
- Produces: `export function quotaView(readings: QuotaAgent[], panel: string): { showInRest: boolean; panel: QuotaRow[] }`. `showInRest` is false whenever no agent is urgent. Rows show the short name, the percent when `used` exists, the plan, each window label, and each detail. Reset text is formatted from `reset5h` or detail `resetAt`. A row click toggles that row between countdown and clock, in memory, default countdown.

- [ ] **Step 1: Failing bun test** `quota_hidden_on_rest_until_urgent` and `countdown_and_clock_share_one_instant`.

- [ ] **Step 2: Run** `bun test src/quota/protocol.test.ts` Expected: FAIL

- [ ] **Step 3: Implement the view model and a panel that uses Bloom's existing panel classes rather than a new visual language.** Add `quotas` to the panel switch. Do not paint quota on the resting island.

- [ ] **Step 4: Re-run the bun test and `bun run build`.** Expected: PASS, build success

- [ ] **Step 5: Commit** message `feat: add the agent quotas panel`

### Task 10: One visibility controller

**Files:**
- Create: `bloom/src/quota/attention.ts`
- Test: `bloom/src/quota/attention.test.ts`
- Modify: `bloom/src/App.tsx` to read the controller instead of setting the open panel directly when an alert arrives

**Interfaces:**
- Consumes: `urgent` from Task 8
- Produces: `export type Attention = { panel: string; alert: string | null; deferred: boolean }` and `export function reduceAttention(prev: Attention, event: AttentionEvent): Attention`

Events: `open-panel`, `alert`, `alert-finished`, `typing`, `dragging`, `pairing`, `connecting`, `clear-defer`. An `alert` while `deferred` is true stores nothing and keeps `panel`. An `alert` while idle sets `alert` and does not change `panel`. `alert-finished` clears `alert` and leaves `panel`. Timer completion is an `alert` with id `timer`.

- [ ] **Step 1: Failing tests** for defer, restore, and timer alert.

- [ ] **Step 2: Run** `bun test src/quota/attention.test.ts` Expected: FAIL

- [ ] **Step 3: Implement the reducer and call it from the notch when quota readings or the timer change.**

- [ ] **Step 4: Re-run the bun test and `bun run build`.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: keep the open panel under quota alerts`

### Task 11: Deferral is wired to real gestures

**Files:**
- Modify: `bloom/src/App.tsx` (focus, drag, bluetooth, wifi)
- Test: `bloom/src/quota/attention.test.ts` if a new event is required; otherwise no new case

**Interfaces:**
- Consumes: `reduceAttention`
- Produces: calls `typing` on text-input focus and `clear-defer` on blur; `dragging` for the duration of a shelf or notch drag; `pairing` while Bluetooth pairing is unanswered; `connecting` while a Wi-Fi connection is in flight.

- [ ] **Step 1: Extend the reducer test** so two alerts during `typing` leave `panel` and `alert` unchanged, then a third alert after `clear-defer` sets `alert`.

- [ ] **Step 2: Run** `bun test src/quota/attention.test.ts` Expected: FAIL on the new case if the reducer does not already do this. If it already passes, the test was late — add a case the reducer does not yet express, then implement. Do not skip the failing run.

- [ ] **Step 3: Wire the four gestures in `App.tsx`.** Do not call `setFocus` on the window except from the text input's click handler.

- [ ] **Step 4: `bun test src/quota/attention.test.ts` and `bun run build`.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: defer notch alerts during typing and pairing`

### Task 12: Optional Overview panel

**Files:**
- Create: `bloom/src/quota/OverviewPanel.tsx`
- Modify: `bloom/src/App.tsx`
- Test: `bloom/src/quota/attention.test.ts` or `protocol.test.ts` for `export function overviewRows(input: { music: boolean; timer: boolean; shelfCount: number; urgent: boolean }): string[]`

**Interfaces:**
- Consumes: the panel switch from Task 9
- Produces: panel id `overview`. Rows are only the activities that are actually on: `Music`, `Timer`, `Shelf`, `Quota`. Opening the notch still uses Bloom's remembered panel, not `overview`.

- [ ] **Step 1: Failing test** `overview_lists_only_live_activities` and `overview_is_not_the_default_panel` (default function returns the existing default, which the test reads from a constant `DEFAULT_PANEL` left unchanged).

- [ ] **Step 2: Run the bun test.** Expected: FAIL

- [ ] **Step 3: Add the panel using the same structure as an existing Bloom panel.**

- [ ] **Step 4: bun test and `bun run build`.** Expected: PASS

- [ ] **Step 5: Commit** message `feat: add an optional activity overview panel`

### Task 13: Focus assist shortcut

**Files:**
- Modify: `bloom/src-tauri/src/commands.rs` (beside `open_battery_saver_settings`)
- Modify: `bloom/src-tauri/src/main.rs` (register the command)
- Modify: `bloom/src/App.tsx` (the button around the `setDndActive` call)
- Test: a Rust unit test only if the URI is a pure function. Add `pub fn focus_assist_uri() -> &'static str` in `commands.rs` or a small module, test equals `ms-settings:quiethours`.

**Interfaces:**
- Consumes: the launch method used by `open_battery_saver_settings`
- Produces: command `open_focus_assist_settings`. The button label and title say `Focus assist` and do not use `dndActive` for an on style.

- [ ] **Step 1: Failing test** `focus_assist_uri_is_quiet_hours`.

- [ ] **Step 2: `cargo test --locked focus_assist_uri_is_quiet_hours`** Expected: FAIL

- [ ] **Step 3: Implement the command and point the button at it.** Remove the local toggle.

- [ ] **Step 4: cargo test and `bun run build`.** Expected: PASS

- [ ] **Step 5: Commit** message `fix: open Focus assist settings instead of a local toggle`

### Task 14: Updates and announcements stay off

**Files:**
- Modify: `bloom/src-tauri/src/main.rs` (the startup `updater::run_startup_check` call)
- Modify: `bloom/src/App.tsx` or the announcement hook call site
- Test: `bloom/src-tauri/src/dock_lifecycle.rs` or a new `bloom/src-tauri/src/excluded.rs`

**Interfaces:**
- Produces: `pub fn run_update_check_on_startup() -> bool` and `pub fn show_announcements() -> bool`, both false. Startup calls the update check only when the first returns true. The announcement hook is mounted only when a function `showAnnouncements(): boolean` returns true.

- [ ] **Step 1: Failing Rust test** asserting both bools are false.

- [ ] **Step 2: cargo test** Expected: FAIL

- [ ] **Step 3: Gate the startup check and the announcement UI.** Leave the updater module on disk.

- [ ] **Step 4: cargo test, cargo check, bun run build.** Expected: success

- [ ] **Step 5: Commit** message `fix: disable Bloom update checks and announcements`

---

## Self-review notes

Spec coverage: protocol Tasks 1–3 and 7–8; dock Tasks 4–5; fullscreen Task 6; panel and bands Tasks 8–9; alerts Tasks 10–11; overview Task 12; Focus assist Task 13; updates Task 14; rollback preserved by not changing window mode except the early flag return. Live desktop checks in the spec's Validation section are not automated tasks. They stay manual after the branch builds.
