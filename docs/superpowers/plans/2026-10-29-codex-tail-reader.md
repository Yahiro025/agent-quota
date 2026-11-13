# Codex Backward Tail Reader Implementation Plan

> **For agentic workers:** Execute task by task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `quota.CodexIn` find a rollout's last accepted `rate_limits` line by reading backwards from the end of the file in fixed windows, so memory per read no longer depends on file size.

**Architecture:** A new file `quota/codex_tail.go` holds a pure scanner over an `io.ReaderAt` plus a thin file wrapper. `CodexIn` in `quota/read.go` calls the wrapper instead of `core.ReadFile` + `core.TextOf` + two `strings.Split` passes. File ranking (`lastEvent`), the 8-file cap, error strings and `ParseCodexLine` are untouched.

**Tech Stack:** Go 1.27, standard library, `agent-quota/internal/core` (`Open`, `Lossy`, `TextOf`).

**Spec:** `docs/verification/2026-10-29-memory-baseline.md` (root cause, §4.3 profile, §4.5 parity prototype). Data facts: `%TEMP%\aqwork\survey\run2.txt`.

## Global Constraints

- Fix Codex memory allocation only. No change to Kiro, Antigravity, Claude, Copilot, Devin, Cursor, `lastEvent`, Bloom, or the running helper.
- Output must equal today's for every real file: 87/87 rollouts on this PC (80 with an accepted line, 7 with none).
- Memory per scanned file bounded by `window + lineCap + parse(lineCap)`, independent of file size. No `GOGC`/`GOMEMLIMIT`/`FreeOSMemory`.
- Open with `core.Open` (share-mode; Codex keeps the file open for writing).
- Error text for read failures unchanged: `Couldn’t read Codex’s logs: %v`.
- `gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green. No commit, merge or deploy without the user's approval.
- Comments explain why; surgical diff.

## Review Focus

Facts from the survey that the obvious design gets wrong or that tests must pin:

1. **Long non-rate-limit lines are normal.** Longest line in the logs is 19,740,009 B; 51 lines exceed 4 MiB, none are rate-limit lines (accepted lines are ≤ 900 B, rejected ones ≤ 758 B). A line longer than `lineCap` must be skipped without being buffered and the scan must continue past it; it must never fail or stop the scan.
2. **Lines straddle windows routinely.** 313 gaps between accepted lines exceed 256 KiB. One real file has its newest accepted line 666,138 B from EOF. Window-boundary cuts must not change the result for any window size.
3. **Rejected needle lines.** The newest line containing `"rate_limits"` can be rejected by `ParseCodexLine`; the scan must continue to an older accepted one (16 such lines exist).
4. **Partial last line.** A file being written may end mid-line; that fragment is a line like any other (rejected if truncated JSON), or accepted if complete without a trailing terminator.
5. **Encodings.** UTF-8 BOM at offset 0 is stripped (as `TextOf` does); invalid UTF-8 inside a line becomes U+FFFD per line; `\r`, `\n`, `\r\n` all end a line. A UTF-16 file has no quoted key in its bytes and yields nothing (documented deviation).
6. **Allocation must not scale with file size even when no line matches.** Lines of 1–200 KB crossing window boundaries are the case that exposed a copy-per-line design; the bounded-IO test must use them, not 100-byte filler.

## Decisions

| Decision | Value | Why |
|---|---|---|
| Needle | `"rate_limits"` with quotes | same as today (66 unquoted occurrences are never parsed) |
| Terminators | `\n` and `\r` each end a line | same as `Split(\n)` then `Split(\r)` |
| Window | 256 KiB | covers 79/80 real files in one read; any size is correct |
| `lineCap` | 64 KiB | 73× the longest accepted line (900 B); bounds carry and parse memory (see revisions) |
| Over-cap line | skipped, never buffered, scan continues | Review Focus 1; documented deviation: a line over the cap that would have parsed is ignored (measured on raw bytes) |
| Size | `Stat().Size()` once | consistent snapshot; later appends ignored |
| Short read / read error | return error → `Fail("Couldn’t read …")` | file truncation is not expected; the poller keeps the last good reading |
| UTF-16 | **dropped**: such a file yields nothing | Opus review: the legacy decode allocated ~0.5 GB at its own 32 MiB cap, the survey found 0 UTF-16 files and Codex writes UTF-8 JSONL. Documented deviation, pinned by a test |
| First line (offset 0) | UTF-8 BOM trimmed, then `core.Lossy`; other lines `core.Lossy` | `TextOf` strips the BOM from the whole text, which only affects the first line |

**Revisions after the Opus pre-implementation review** (verified by reproduction):

- `lineCap` is **64 KiB**, not 1 MiB: parsing a line costs 168–381× its length, so a 1 MiB cap allowed ~400 MB per parse. 64 KiB is 73× the longest accepted line (900 B).
- The first design (join each window-crossing line into a new slice, prepend the carry by copying) allocated **340 MB** on a 300 MB no-match file of 1–200 KB lines (old code: 630 MB), so it was not bounded by the window. Fixed: one `min(lineCap,size)` buffer filled from the right, no copy when no carry is pending, over-cap lines never held. Pinned by a bounded-IO test with 1–200 KB lines.
- Files of 0 or 1 byte must give `ok=false, err=nil` (an empty newest rollout ranks first by mtime); there is no BOM probe read any more, which removes the failure.
- The parity harness runs on a frozen snapshot, not hardlinks, because Codex appends to live files.
- Not adopted: SIMD fast path (`bytes.IndexByte` + `Contains`, ~4× faster for long no-match scans); the per-byte scan matches the old code's time and the case is rare. Revisit only if the benchmark shows a regression. No scan-distance guard: real gaps reach 19.7 MB.

---

### Task 1: Tail scanner

**Files:**
- Create: `quota/codex_tail.go`
- Test: `quota/codex_tail_test.go`

**Interfaces:**
- Produces:
  - `const codexTailWindow = 256 << 10`, `const codexLineCap = 64 << 10`
  - `func lastRateLimits(r io.ReaderAt, size int64, now time.Time, window, lineCap int) (Reading, bool, error)` — last line in file order that contains `"rate_limits"` and that `ParseCodexLine(line, now)` accepts; `false` if none.
  - `func rolloutLimits(path string, now time.Time) (Reading, bool, error)` — opens with `core.Open`, stats, calls `lastRateLimits(f, size, now, codexTailWindow, codexLineCap)`.

- [ ] **Step 1: Write the failing tests** in `quota/codex_tail_test.go` (all call `lastRateLimits` through a `bytes.Reader` unless noted). Use a test-local oracle `oracleLast(text, now)` that is the current algorithm: `core.TextOf`, split on `\n` then `\r`, keep the last `ParseCodexLine`-accepted line containing `"rate_limits"`.
  - `TestTailLastAcceptedLineWins` — three accepted lines with distinct `used_percent`; result is the third.
  - `TestTailRejectedNewestFallsBack` — newest needle line has `"rate_limits":null` or a string `used_percent`; result is the older accepted line.
  - `TestTailTerminators` — same content with `\n`, `\r\n`, bare `\r`, and mixed endings gives the same `Reading`.
  - `TestTailPartialLastLine` — (a) valid line then truncated JSON, no terminator: returns the valid line; (b) valid line with no trailing terminator: returns it.
  - `TestTailWindowSweep` — for every `window` in 1..300 and `lineCap` large, result equals the oracle on a fixed multi-line file that has lines crossing every boundary; plus a seeded random generator (≥ 200 files of mixed accepted, rejected-needle, filler, long filler, mixed terminators) compared with the oracle for windows {1, 7, 64, 4096, codexTailWindow}.
  - `TestTailOverCapLineSkipped` — with `lineCap=64`: a 10 KiB filler line between and after accepted lines is skipped and the scan continues to the newest accepted line before it; an accepted line longer than the cap is ignored and the older one returned.
  - `TestTailNoLimits` — empty file, terminator-only file, file with needle only in rejected lines → `ok=false`, `err=nil`.
  - `TestTailBOMAndInvalidUTF8` — UTF-8 BOM before a first-line accepted line is accepted; invalid UTF-8 bytes inside an accepted line match the oracle.
  - `TestTailBoundedIO` — a generated `io.ReaderAt` of 1 GiB virtual size (deterministic filler, no allocation) with an accepted line 600 KB before EOF: result correct; total bytes requested ≤ 4 × `codexTailWindow`; `runtime.MemStats.TotalAlloc` delta < 8 MiB. Second case: 256 MiB virtual file with no needle → `ok=false`, same alloc bound. Third case: a 64 MiB virtual final line with no terminator → skipped with the same alloc bound.
  - `TestTailReadError` — a `ReaderAt` that fails returns the error; a short read returns `io.ErrUnexpectedEOF`.
  - `TestRolloutLimitsUTF16` — UTF-16LE and BE files holding an accepted line return `ok=false, err=nil` (documented deviation), and a 40 MiB UTF-16 file allocates under 8 MiB.

- [ ] **Step 2: Run to verify they fail:** `go test ./quota -run 'TestTail|TestRolloutLimits' -count=1` → compile error, undefined `lastRateLimits`.

- [ ] **Step 3: Implement `lastRateLimits` and `rolloutLimits` in `quota/codex_tail.go`.** Algorithm (not determined by the signature):
  - `pos := size`; loop reading `n := min(window, pos)` bytes at `pos-n` into one reused buffer; short read → `io.ErrUnexpectedEOF`.
  - `end := len(chunk)`; scan `i` from `len(chunk)-1` down; at each `\n`/`\r` the line is `chunk[i+1:end] + carry`: if `skipping` or its length exceeds `lineCap`, drop it; otherwise `bytes.Contains` the needle, convert with `core.Lossy`, call `ParseCodexLine`, return on the first accepted. Then `carry = nil`, `skipping = false`, `end = i`.
  - After the chunk, the unterminated head `chunk[:end]` becomes `carry = chunk[:end] + carry` (a fresh copy, the buffer is reused); if that would exceed `lineCap`, set `skipping = true` and `carry = nil`.
  - When `pos == 0`, the remaining head plus carry is the first line: evaluate it unless skipping, using `core.TextOf` instead of `core.Lossy`.
  - Carried bytes live in one `min(lineCap,size)` buffer filled from the right; a line is joined in place, or used where it lies when nothing is carried. Over-cap lines set a `skipping` state that drops their remaining pieces. The line that starts at offset 0 is what is left in the buffer at the end; it has a UTF-8 BOM trimmed.
  - `rolloutLimits`: `core.Open`, `Stat`, then the scan with the shipped constants.

- [ ] **Step 4: Run to verify they pass:** `go test ./quota -run 'TestTail|TestRolloutLimits' -count=1 -race` → PASS. Record the real output.

---

### Task 2: Use it in `CodexIn`

**Files:**
- Modify: `quota/read.go:209-229` (the `for _, f := range files[:min(len(files), 8)]` body)
- Test: `quota/codex_tail_test.go`

**Interfaces:**
- Consumes: `rolloutLimits(path string, now time.Time) (Reading, bool, error)`.

- [ ] **Step 1: Write the failing test** `TestCodexInLargeRolloutBoundedAlloc` — temp `CODEX_HOME` with one ~64 MiB rollout (skip under `testing.Short()`), accepted line ~600 KB before EOF; `CodexIn` returns the expected `Used`/`Detail`; `TotalAlloc` delta < 16 MiB. Add `BenchmarkCodexInLarge` with `b.ReportAllocs()`.
- [ ] **Step 2: Run:** `go test ./quota -run TestCodexInLargeRolloutBoundedAlloc -count=1` → FAIL (allocs ≈ 3× file size).
- [ ] **Step 3: Replace the body** with: call `rolloutLimits`; on error `return Fail(fmt.Sprintf("Couldn’t read Codex’s logs: %v", err))`; on `ok` return the reading. Keep the comment about Codex writing the file, adding why the read is a backward tail scan (rollouts reach hundreds of MB). Remove imports that become unused.
- [ ] **Step 4: Run:** `go test ./quota -count=1` (all existing Codex tests included) → PASS.

---

### Task 3: Parity and benchmark gate

**Files:** none in the repo (uses `%TEMP%\aqwork`).

- [ ] **Step 1: Parity over real data, outside the repo.** `%TEMP%\aqwork\parity`: for each real `rollout-*.jsonl`, build a temp `CODEX_HOME` whose `sessions\` holds one hardlink (`os.Link`, same volume; copy if it fails) to it, then call the public `quota.CodexIn(home, fixedNow)` from two binaries, one built against the frozen baseline tree `%TEMP%\aqbase` and one against the repo, and compare `Used`, `Detail`, `Plan`, `Reset5h`, `ObservedAt` and the rest of `Reading` for equality. Include the 7 files with no limits and the file with the 666 KB gap. Expected: `files=87 same=87 diff=0`. Never read or print log content.
- [ ] **Step 2: Synthetic before/after.** `powershell -File %TEMP%\aqwork\bench\harness\run.ps1 -Runs 3 -N 30`. Record latency median, `TotalAlloc`/call, peak `HeapInuse`, peak private bytes, and that `Used`/`Detail` are identical.
- [ ] **Step 3: Lifecycle harness.** Rebuild `aq.exe` from the candidate tree and run `%TEMP%\aqprobe\harness\harness.exe` (10 min) on a copy; compare phases with the baseline table in the report.
- [ ] **Step 4: Full suite.** `gofmt -l .` (empty), `go vet ./...`, `go test ./...`, `go run . --print` Codex line unchanged. Record actual output; if anything fails, say so.
