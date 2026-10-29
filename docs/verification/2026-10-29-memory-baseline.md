# Memory baseline and reduction plan — agentquota helper and Bloom (2026-10-29)

**Historical baseline:** Sections 1–10 describe the original read-only investigation
and its proposed prototype, not the current implementation or a completed deployment.
See the post-implementation status below before using the plan or acceptance claims.
No product code was changed during that original investigation. Probes and harnesses live in
`%TEMP%\aqprobe` (outside the repo) and import the real `quota` package through a
`replace` directive.

Labels: **confirmed** = measured and attributed with profiling evidence;
**suspected** = consistent with evidence but not isolated; **unverified** = plausible,
not measured.

## 1. Summary

| Finding | Label | Evidence |
|---|---|---|
| `agentquota.exe --quota-service` holds ~1.26–1.28 GB private memory for its whole life | confirmed | live helper PID 34480: 1,277 MB commit / 1,260 MB private WS at 11 min and 16 min; harness reproduction: 1,283 MB |
| The cause is `CodexIn` reading whole Codex rollout files (363 MB here) into memory every 30 s | confirmed | alloc profile: 99.7 % of 1.22 GB per call is `io.ReadAll` (837 MB) + `core.Lossy` string copy (364 MB) |
| It is not a leak: live heap after GC is 0.4 MB; the GC heap goal and retained spans keep ~1.2 GB mapped because the same garbage is re-created every tick | confirmed | probe `liveAfterGC=0.4MB`; steady state `heapSys=1232MB heapReleased≈16MB nextGC=728MB` for 6 cycles; gctrace below |
| A bounded backward tail scan gives byte-identical readings for every rollout on this PC while reading ≤ 0.8 MB per file | confirmed (prototype) | parity 87/87 files, 20.7 MB read vs 814 MB, 0.02 s vs 1.1 s |
| Each Antigravity read (every 5 min) starts the user's configured MCP servers through `agy` | confirmed | process tree: `agentquota → agy → cmd → node → cmd → node`, `agy → uvx → uv → mcp-for-blender → python → python` |
| Bloom + WebView2 tree is ~270 MB private, of which GPU process ~101 MB | confirmed (measured), attribution per window unverified | per-process table below |

The single change with the largest effect is replacing the whole-file Codex read with a
bounded tail read. No resident figure after the fix is predicted here; it must be measured
(see §8).

## 2. Architecture and memory model

```
bloom.exe (Tauri 2, Rust)                          ← user-visible notch; owns the helper
├─ msedgewebview2.exe  browser                     ← WebView2 (Chromium) process model
│  ├─ gpu-process, network utility, storage utility, crashpad
│  └─ renderer ×3                                  ← windows: main (notch), settings, overlay — all created at startup (tauri.conf.json)
└─ agentquota.exe --quota-service (Go 1.27)        ← this repo; JSON lines on stdin/stdout (quota/service.go)
   ├─ gh.exe            every 30 s   (Copilot, 15 s deadline)
   ├─ devin.exe         per Devin read (seen at cold start; role not traced)
   ├─ kiro-cli.exe → kiro-cli.exe     every 5 min (25 s deadline)
   └─ agy.EXE → user MCP servers      every 5 min (Antigravity /usage)
```

Memory models in play:

- **Go helper.** Single process, GC'd heap. Default `GOGC=100`: the next GC triggers when
  the heap reaches 2× the live heap at the previous GC. Freed spans are returned to the OS by
  the background scavenger gradually. A process that allocates a large transient buffer on a
  fixed tick therefore keeps that much mapped indefinitely, even with almost nothing live.
  Task Manager "Memory" ≈ private working set; "commit" = `PrivateMemorySize64`.
- **Child CLIs.** Separate processes inside a job object (`internal/proc.Spawn`); their
  memory is transient and returned on exit. Verified: no helper-spawned process outlived its
  read.
- **WebView2.** Chromium multi-process. Each WebView2 window is a renderer; the browser, GPU,
  network and storage processes are shared. Memory here is mostly framework/runtime
  overhead, not app state.

Metrics used: *WS* = working set (includes shared/image pages); *Private WS* = Task Manager
"Memory"; *Commit* = private bytes charged against the page file. For the Go helper, Go
runtime numbers (`HeapInuse`, `HeapSys`, `HeapReleased`, `NextGC`) are read via
`runtime.ReadMemStats` in the probe and `GODEBUG=gctrace=1` in the harness.

## 3. Profiling strategy

| Question | Tool | Why |
|---|---|---|
| Which processes, how much of each metric | `Win32_Process` tree walk + `Get-Process` + `Win32_PerfFormattedData_PerfProc_Process.WorkingSetPrivate` | separates WS, private WS and commit per process |
| What inside the Go helper allocates | isolated probe calling each reader, `runtime.MemStats` sampler (2 ms), `runtime/pprof` allocs profile (`-sample_index=alloc_space`) | line-level attribution without touching the live helper |
| Leak vs. retention | `runtime.GC()` then `HeapAlloc` ("live after GC"); repeated-cycle steady-state run | distinguishes live data from GC/scavenger retention |
| Lifecycle (cold/idle/intensive/recover) | standalone `aq.exe --quota-service` built from the repo, driven by a Go harness with scripted stdin; `GODEBUG=gctrace=1` | reproducible, does not disturb Bloom's helper (AGENTS.md: leave it running) |
| Child processes | ToolHelp snapshot each 0.5 s, parent chain, peak WS by image | attributes transient children correctly (several `agy`/`node` on this PC are not ours) |
| Fix viability | parity harness: current full scan vs bounded tail scan on every rollout | proves identical output before any code change |

Rejected as primary tools: VMMap/WPA (not installed; Go heap is opaque to them anyway);
attaching pprof to the live helper (no debug endpoint exists, and adding one is a code change).

## 4. Evidence

### 4.1 Live process inventory (Bloom started 04:50:13, sampled at ~11 min)

| PID | Process | WS MB | Private WS MB | Commit MB | Peak WS MB |
|---|---|---:|---:|---:|---:|
| 34480 | agentquota (helper) | 1,289.8 | **1,260.2** | **1,277.1** | 1,404.2 |
| 20704 | bloom | 102.8 | 28.9 | 33.3 | 117.2 |
| 49396 | webview2 browser | 134.9 | 33.8 | 43.0 | 135.6 |
| 41308 | webview2 gpu | 158.2 | 101.3 | 130.9 | 207.5 |
| 33140 | webview2 renderer | 112.9 | 50.2 | 61.8 | 310.2 |
| 50676 | webview2 renderer | 74.9 | 23.5 | 34.1 | 77.4 |
| 39732 | webview2 renderer | 60.7 | 17.0 | 26.4 | 60.7 |
| 28708 | webview2 network | 47.1 | 9.2 | 14.4 | 47.6 |
| 38260 | webview2 storage | 21.4 | 3.4 | 8.1 | 21.4 |
| 26724 | webview2 crashpad | 16.0 | 2.4 | 3.7 | 21.4 |
| | **total** | 2,018.7 | **1,529.9** | 1,632.8 | |

The helper is 82 % of the tree's private memory. Task Manager's screenshot (agentquota
1,228.4 MB) agrees. The `agy` entries in Task Manager (341–436 MB) are the user's own
interactive `agy` (parent `powershell.exe`), not the helper's.

Helper over time (live): 1,277.0 MB commit at 11:11 uptime, 1,277.3 MB at 16:04; flat. CPU
8.0 s → 11.1 s over 293 s ≈ 1.1 % of one core.

### 4.2 Per-reader allocation (probe, one call each, same data)

| Reader | Time | Allocated | Peak heap in use | Live after GC |
|---|---:|---:|---:|---:|
| Codex (`CodexIn`) | 564 ms | **1,217.5 MB** | **838.2 MB** | 0.4 MB |
| Codex, second call | 423 ms | 1,217.4 MB | 838.2 MB | 0.4 MB |
| Cursor | 442 ms | 0.3 MB | 1.4 MB | 0.6 MB |
| Claude | 28 ms | 0.9 MB | 2.1 MB | 1.1 MB |

Copilot, Kiro, Antigravity and Devin do their heavy work in child processes (§4.6) or in
bounded HTTP reads (`io.LimitReader(…, 16<<20)`).

### 4.3 Allocation profile of one `CodexIn` call

```
      flat  flat%        cum   cum%
  837.26MB 68.69%   837.26MB 68.69%  io.ReadAll                 ← core.ReadFile (quota/read.go:211)
  363.86MB 29.85%   363.86MB 29.85%  core.Lossy                 ← core.TextOf string copy (read.go:217)
   13.74MB  1.13%    14.99MB  1.23%  core.(*parser).object      ← ParseCodexLine on rate_limits lines
                   1218.40MB   100%  quota.CodexIn
```

`io.ReadAll` grows its buffer by appending, so a 363 MB file costs ~2.3× its size in
allocations; `Lossy` then copies the whole buffer into a `string`. The two `strings.Split`
passes are negligible by comparison (6,871 lines).

Data that drives it: newest rollouts are 363.1 MB and 142.4 MB
(`~/.codex/sessions/2026/10/28/rollout-…`). `CodexIn` ranks files by last event and reads
up to 8 of them whole until one has a `rate_limits` line.

### 4.4 Why memory never comes back (steady state, Codex every 5 s, default GC)

```
cycle 0 commit=1256MB heapSys=1232MB heapReleased=28MB heapAlloc=379MB nextGC=728MB
cycle 1 commit=1268MB heapSys=1232MB heapReleased=16MB heapAlloc=379MB nextGC=728MB
...
cycle 5 commit=1268MB heapSys=1232MB heapReleased=16MB heapAlloc=379MB nextGC=728MB
```

The 363 MB string is live at the GC inside the read, so the heap goal settles at ~728 MB and
the heap grows to ~1.2 GB during each read. Between reads the scavenger returns almost
nothing (16 MB) before the next tick recreates the same garbage. Same plateau as the live
helper (1,268 vs 1,277 MB).

### 4.5 Parity of a bounded tail scan (prototype in the probe, not product code)

Fixed 256 KB windows read backwards; partial line carried across windows; carry capped at
16 MB (overlong lines skipped, counted); `\n` and `\r` both end a line; the newest line that
contains `"rate_limits"` and that `ParseCodexLine` accepts wins, else keep walking back.

```
files=87 same=87 diff=0 | full: 1.1s over 814MB | tail: 0.02s, read 20.7MB total,
max per file 0.8MB, overlong lines skipped=0
```

A forward streaming scan (bufio, 1 MB buffer) was also measured: peak heap 25 MB but still
reads the whole file (383 ms). The tail scan is strictly better here.

### 4.6 Helper child processes (5-minute trace, live helper)

| Child | Parent | Cadence | Peak WS MB | Commit MB | Life |
|---|---|---|---:|---:|---|
| gh.exe (+conhost) | agentquota | 30 s | 43.9 | 64.4 | ~1–2 s |
| kiro-cli.exe | agentquota | 5 min | 55.3 | 90.9 | ~3 s |
| kiro-cli.exe | kiro-cli | 5 min | 327.5 | 311.0 | ~2 s |
| agy.EXE | agentquota | 5 min | 203.2 | 185.9 | ~3 s |
| node.exe (MCP) | agy → cmd | 5 min | 130.2 / 236.9 | 153 / 280 | ~2 s |
| uv / mcp-for-blender / python (MCP) | agy → uvx | 5 min | 38.6 / 9 / 65.3 | | ~2 s |

All exited with their parent; no helper-spawned process remained afterwards. Harness cold
start: children peaked at 920 MB combined WS ~3.6 s after `hello`, dropping to 0 by 7 s.
This cost is transient and lives outside the helper's address space. Neither `agy --help`
nor `kiro-cli chat --help` offers a per-run switch to skip MCP startup (`agy mcp disable`
is global and would change the user's config).

### 4.7 Bloom / WebView2 observations

- Three renderers for three windows (main notch, settings, overlay); all are declared in
  `tauri.conf.json` and so are created at startup even though settings and overlay are
  hidden. Which renderer is which window has not been mapped.
- Renderer 33140 (largest, most likely the notch) used ~5.6 % of one core and `bloom.exe`
  ~4 % over a 45 s idle-ish sample while its commit swung 67–103 MB (JS GC churn). Cause
  unverified; media was playing and Bloom has an FFT visualiser (`rustfft`). CPU, not RAM.
- No WebView2 memory tuning is in use (no `MemoryUsageTargetLevel`, `TrySuspend`, or
  browser args).

## 5. Benchmark (standalone helper from the repo, scripted harness)

Run: `aq.exe --quota-service` built from this repo at 05:11, 600 s scripted run, one `refresh all` every 10 s from 300 s to 420 s. Samples every 500 ms (`samples.csv`, 1,108 rows). Commit = `PrivateMemorySize64`. Child WS = sum of all live descendants. Run length was 10 minutes, not the one hour in §9.

| Phase | Time (s) | Helper commit MB (min–max, last) | Helper WS max MB | Child WS max MB | Helper CPU used (s) |
|---|---|---|---|---|---|
| Cold start | 0–60 | 34–1,283 (1,283) | 1,252 | 919 | 1.68 |
| Idle | 60–300 | 1,266–1,398 (1,277) | 1,368 | 53 | 1.65 |
| Intensive (refresh every 10 s) | 300–420 | 1,268–1,443 (1,272) | 1,389 | 1,220 | 3.36 |
| Recover | 420–600 | 1,267–1,286 (1,272) | 1,255 | 54 | 5.08 |

Gc behaviour (`gctrace.txt`, 50 cycles): the heap goal is 1,201 MB; each read grows the heap to ~1,200 MB, then it drops to ~364 MB live at the end of the GC cycle. The 364 MB live figure is the whole rollout text held during the parse, so the reading itself is what sets the peak.

Peak child WS by image (maximum seen for each image, not simultaneous): node.exe 406 MB, agy.EXE 279 MB, kiro-cli.exe 257 MB, uv.exe 49 MB, devin.exe 49 MB, gh.exe 45 MB, python.exe 75 MB, mcp-for-blender.exe 9 MB, uvx.exe 9 MB.

Open items:
- **Copilot read failed in the run** ("Copilot's answer couldn't be read"). `gh auth status` reports a logged-in account, so sign-in is not the cause. Not yet investigated; the failure is outside the memory question.
- The one-hour acceptance run in §9 has not been done.

## 6. Root cause

1. **Confirmed.** `quota/read.go:209-226` reads up to eight Codex rollout files whole on
   every 30 s tick: `core.ReadFile` (`io.ReadAll` growth) then `core.TextOf` (full string
   copy) then `strings.Split`. With a 363 MB rollout that is ~1.22 GB allocated and ~840 MB
   peak heap per tick. Only the last `rate_limits` line is used.
2. **Confirmed.** Go's GC pacing plus periodic re-allocation keeps the high-water mark
   mapped: the live heap after GC is under 1 MB, but `HeapSys` stays at ~1.2 GB because the
   garbage is recreated before the scavenger can return it.
3. **Confirmed, transient.** Every five minutes the Antigravity read spawns `agy`, which
   boots the user's MCP servers (~700 MB combined WS for ~3 s); Kiro briefly uses ~380 MB.
4. **Minor, confirmed by reading.** `lastEvent` opens every rollout file (87 here) each tick
   to read its last 8 KB, ~0.7 MB allocation and 87 opens per 30 s. Not a RAM problem.

## 7. Optimisation opportunities (ranked)

| # | Change | Expected RAM effect | CPU / latency | Effort | Risk | Confidence |
|---|---|---|---|---|---|---|
| 1 | Bounded backward tail scan of each Codex rollout (fixed window, capped carry) | per-read allocation from ~1.2 GB to < 1 MB on this data (measured on the prototype); resident effect to be measured | Codex read 0.4–0.56 s → ~1 ms; helper CPU drops correspondingly | small, one function | low with parity test | high |
| 2 | Within #1, stop scanning older files early on success (already the case) and keep the 8-file cap | bounds worst case to 8 × window when no file has limits | — | trivial | low | high |
| 3 | Cache `lastEvent` per path keyed by size (append-only logs grow) | negligible RAM; fewer opens | small CPU win | small | low (Windows mtime caveat already noted in code; size still changes) | medium |
| 4 | Antigravity: investigate a way to run `/usage` without starting MCP servers (e.g. an isolated config dir or a documented flag in a later agy) | transient ~700 MB every 5 min | faster read | unknown | medium (must not alter user config) | unverified |
| 5 | Kiro: try `--agent` with a minimal agent that has no MCP servers | transient ~300 MB every 5 min | — | small | medium (needs Kiro agent config) | unverified |
| 6 | Bloom: create the settings webview lazily (on first open), destroy on close | one renderer, size unmapped (17–50 MB range seen) | first open slower | medium | medium (settings sync, mica) | suspected |
| 7 | Bloom: set WebView2 `MemoryUsageTargetLevel=Low` on hidden webviews | unknown | wake cost | medium | medium | unverified |

Rejected because they hide cost instead of removing it: `GOMEMLIMIT`, lowering `GOGC`,
`debug.FreeOSMemory()` after reads, periodic restarts of the helper. They would trade CPU
for RSS while still allocating ~1.2 GB every 30 s.

## 8. Implementation plan (small, independently testable steps)

This is the original proposal, retained as history. Its long-line skip and UTF-16
whole-file fallback are not the current design; see the post-implementation status.

1. **Test first.** Add `quota` tests for a rollout tail reader: last valid line wins;
   unparseable newest line falls back to an earlier one; `\r`, `\n`, `\r\n` endings; partial
   final line (file being written); line spanning window boundary; overlong line beyond the
   cap is skipped; file with no `rate_limits`; UTF-8 BOM. Use small windows in tests to
   exercise boundaries.
2. **Implement** `lastRateLimits(path, now) (Reading, bool)` in `quota/read.go` using
   `core.Open` + `ReadAt` with a fixed window (256 KB) and capped carry. Replace the
   `core.ReadFile`/`TextOf`/`Split` block in `CodexIn`. Keep ranking and the 8-file cap.
   Note in a comment why (files reach hundreds of MB). Preserve `TextOf` behaviour: a UTF-8
   BOM at offset 0 is stripped, invalid UTF-8 is made lossy on the parsed line only. If a
   UTF-16 BOM is found at offset 0, fall back to the current whole-file path (none of the 87
   local rollouts has one; whether Codex can write UTF-16 is unverified).
3. **Parity check** (one-off, kept as a test helper or in `docs/scratch`): old vs new over
   all local rollouts, identical `Reading` per file.
4. **Benchmark test**: `testing.B` with a synthetic 300 MB rollout, `b.ReportAllocs()`;
   assert allocations per op stay under a fixed bound independent of file size.
5. **Re-run the harness** (§9) on the same rollout set and compare with §5.
6. Optional follow-ups, each separate: `lastEvent` size-keyed cache (#3); Antigravity/Kiro
   MCP investigation (#4, #5); Bloom webview items (#6, #7) with their own baseline.

## 9. Acceptance criteria

- `gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green, new tail-reader tests
  green.
- Parity: identical Codex `Reading` for every rollout file on this PC (87 today).
- Per-call allocation of `CodexIn` bounded by the window and carry cap, not by file size
  (benchmark with 300 MB synthetic file).
- Harness on the same data: helper commit and private WS after cold start, idle, intensive
  and recover phases reported side by side with §5. Resident memory after one hour of the
  live helper compared with the 1,277 MB baseline. No figure is promised in advance.
- No functional regression: `go run . --print` shows the same Codex line as before; Bloom
  notch shows Codex usage and reset unchanged; refresh latency for Codex not worse.
- Rollout sizes recorded alongside each measurement (they change as Codex writes).

## 10. Reproduce

```
# probe (outside the repo)
%TEMP%\aqprobe\probe.exe readers   # per-reader allocation
%TEMP%\aqprobe\probe.exe prof      # writes alloc.pprof; go tool pprof -sample_index=alloc_space -top probe.exe alloc.pprof
%TEMP%\aqprobe\probe.exe steady    # GC retention across cycles
%TEMP%\aqprobe\probe.exe alt       # full vs stream vs tail on the largest rollout
%TEMP%\aqprobe\probe.exe parity    # full vs bounded tail on every rollout
# harness: standalone service built from the repo, 10 min scripted lifecycle
go build -o %TEMP%\aqprobe\aq.exe .
%TEMP%\aqprobe\harness\harness.exe %TEMP%\aqprobe\aq.exe   # samples.csv, events.txt, gctrace.txt
```

## Post-implementation status

The unmerged, undeployed implementation is in `quota/codex_tail.go` and
`quota/codex_stream.go`. It keeps a 64 KiB ordinary-record fast path, uses a
bounded streaming parser for larger records, and scans UTF-16 without a
whole-file fallback. The original proposal's skipped-long-line behavior was a
regression and was removed. See
[`2026-10-29-codex-legacy-semantics.md`](2026-10-29-codex-legacy-semantics.md)
for the current compatibility contract and verification details.

On a frozen 88-file snapshot, the legacy and current readers produced matching
readings for every file. The per-file allocation total was 2,792.7 MiB versus
30.7 MiB, with maximum sampled heap-in-use of 1,201.6 MiB versus 1.5 MiB.
These are per-read measurements, not a service lifecycle or private-working-set
comparison. A delayed independent reviewer additionally reported 95/95 matching
current local rollouts and no differences across 300,000 mutated test lines;
those reviewer-run scratch results were not independently rerun here.

The earlier standalone lifecycle runs selected a small rollout rather than the
380 MB file and had measurement defects, so they do not establish the resident
memory improvement. They are superseded by the corrected lifecycle measurements
in "Final results" below. Production Bloom behavior remains unverified.

## Final results (2026-10-29, evening)

Status: implemented and verified in the working tree. **Not committed, merged or
deployed.** The running production helper (PID 34480) and Bloom (PID 20704) were
never touched; every measurement used a separate copy of the service.

### What changed

| File | Change |
|---|---|
| `quota/read.go` | `CodexIn` calls `rolloutLimits` per file. `git diff HEAD`: +5 / -16. Ranking, the 8-file cap and error text unchanged. |
| `quota/codex_tail.go` (new, 282 lines) | Backward scan in 256 KiB windows, 64 KiB line buffer, UTF-16 path, rescan up to 3 times if the file shrinks mid-scan. |
| `quota/codex_stream.go` (new, ~640 lines) | Bounded streaming parser for lines over 64 KiB (UTF-8 and UTF-16) that reproduces `core.ParseJSON` + `ParseCodexLine` + `CodexPrimaryReset` semantics. |
| six `quota/codex_*_test.go` files | About 3,100 lines of tests (see cost below). |

The first design (skip lines over the cap) was a real regression: a valid newer record
over 64 KiB would lose to an older reading. It was replaced by the streaming parser. The
first carry design also allocated about the file size on long-line files (340 MB on a
300 MB file) and was replaced by a fixed buffer filled from the right.

### Lifecycle A/B on the 380 MB rollout (corrected harness, 13 min each, one round)

Workload: `--quota-service` with `CODEX_HOME` holding only the 380,766,200-byte rollout
(sha256 `F88C1954…52B9`, unchanged before and after). Binaries differ only in
`quota/read.go`. Criteria were written before the runs (`%TEMP%\aqwork\lifecycle\CRITERIA.md`).

| | legacy | new |
|---|---|---|
| Private working set, median (cold / idle / burst) | 1,246 / 1,261 / 1,251 MB | 11.7 / 14.2 / 12.9 MB |
| Commit, max (cold / idle / burst) | 1,282 / 1,632 / 1,283 MB | 60.6 / 63.6 / 63.0 MB |
| OS peak working set | 1,658.6 MB | 37.0 MB |
| Helper CPU, user+kernel, idle phases (600 s) | 27.17 s | 0.83 s |
| Codex refresh latency p50 / p99 | 674.4 / 720.6 ms | 1.7 / 2.5 ms |
| Max GC heap goal | 1,202 MB | 9 MB |
| Complete Codex reading | one reading | byte-identical |

Criteria 1-4 passed. Criterion 5 as written ("handles max - min <= 50 over the run")
fails for both binaries because handles ramp from about 6 to about 500 at start-up; it
was badly specified. Growth after start-up is +5 (legacy) and +11 (new).

### One-hour stability run (new binary, same workload)

| Criterion (written beforehand) | Result |
|---|---|
| Alive all hour, clean shutdown, fixture unchanged | pass (exit clean, sha equal) |
| One unique `ok` Codex reading, no failed/stale snapshot | pass (1 reading, 531 snapshots, one initial `waiting`) |
| Private working set max <= 150 MB, trend <= 10 MB/h | pass (max 17.2 MB, trend -0.7 MB/h) |
| Commit max <= 200 MB | pass (64.6 MB) |
| Handles: max after 5 min minus first-5-min max <= 50 | pass (+38); see note |
| Refresh p99 <= legacy median (674 ms) | pass (p99 2.6 ms, max 2.7 ms, 132 refreshes, 0 timeouts) |

Handles stayed in a band, not a trend: per-15-minute maxima 534, 547, 457, 453 and a
negative median slope. About 90 handles dropped around the 35-minute mark; the cause
was not investigated.

### Other evidence

- Real-data parity, frozen 88-file snapshot (814 MiB), per-file `CodexIn`: 88/88 identical;
  allocation 2,792.8 -> 30.7 MiB, peak heap 1,201.7 -> 1.6 MiB.
- Independent Opus review (own fuzzer, seed 20261029): 20,000 files vs the legacy
  algorithm, 0 mismatches, including 1,571 accepted lines over 64 KiB and 2,006 UTF-16
  files; 200 multi-file `CodexIn` homes, 0 differences; races (append, rewrite, truncate,
  rename, overwrite) produced 0 panics and 0 errors.
- Mutation testing: a previous reviewer's 20 mutants plus 17 more by the test author;
  four survivors were closed by `codex_gaps_test.go`; one (`rolloutLimits` wrapper) is
  untestable because `core.Open` returns an `*os.File`.
- `gofmt -l .` empty, `go vet ./...` clean, `go test ./... -count=1` passes in all 9 packages.

### Costs and behaviour differences (must be known before merging)

- **CPU is worse for line shapes your logs do not contain.** Lines over 64 KiB that carry
  the key: 3-7x slower than legacy; over-64 KiB lines without the key: 1.3-1.5x; rejected
  key lines under 64 KiB: 1.3-2.2x. Locally, accepted lines are at most 900 B and none is
  over 64 KiB; the 19.7 MB line is a non-rate-limit line.
- **I/O amplification:** up to 3.0x bytes read when an oversized line carries the key
  (2.0x without the key). The realistic tail case reads 0.01x of the file. Cold-disk
  behaviour was not measured.
- **Total allocation per call is bounded in peak, not in total** for adversarial inputs
  (many rejected candidate lines), but was never above legacy in any class tested.
- **Behaviour differences:** a short read that persists through 3 rescans is an error
  where legacy returns what it read; a `ReaderAt` returning `(2, io.EOF)` for a 2-byte BOM
  file errors (an `*os.File` never does); bytes appended after the size is taken wait
  for the next poll.
- **Test time:** `go test ./quota` rose from about 12 s to about 26 s.
- **Production code:** about 920 new lines replaced 16.

### Not verified

Cold-disk reads; production Bloom integration; deployment; more than one A/B round
(order effects uncontrolled, though the gap is about 100x); `-race` (no cgo here);
number parity for exponents of 6 or more digits on mantissas over 100K digits beyond the
pinned case; the cause of the handle drop mid-run.

### Merging

The working tree also holds unrelated uncommitted changes under `quota/` (`copilot.go`,
`devin.go`, `antigravity.go`, `claude_snapshot.go`). Stage only `quota/read.go`, the new
`quota/codex_*.go` files, the 2026-10-29 documents, and the plan in
`docs/superpowers/plans/`.

### Reproduce

```
%TEMP%\aqwork\lifecycle\lifecycle.exe -exe <aq-new|aq-legacy>.exe -codexhome %TEMP%\aqwork\lifecycle\home -out <dir> -total 780 -cold 60 -idle 240 -burst 60 -burstevery 5 -fixture <rollout>
powershell -File %TEMP%\aqwork\parity\run.ps1 -Sessions %TEMP%\aqwork\snapshot\sessions
```
