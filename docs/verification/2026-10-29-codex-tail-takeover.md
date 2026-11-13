# Codex tail reader takeover verification

Status: SUPERSEDED, kept as history. The HOLD below was correct when written: the tail
reader skipped valid records larger than 64 KiB. Lines over 64 KiB now go through a
bounded streaming parser (`quota/codex_stream.go`, contract in
`2026-10-29-codex-legacy-semantics.md`), and the lifecycle mismatch described here is
replaced by the forced-large A/B in `2026-10-29-memory-baseline.md`. Merge and deploy
still need the user's approval.

Original status: HOLD. The tail reader still skips valid records larger than 64 KiB.
Do not merge or deploy on the basis of these results.

## Findings

- The old `core.TextOf` accepted UTF-16LE and UTF-16BE BOM files. The tail reader
  now reads both using bounded buffers, and preserves the old decoder's treatment
  of an incomplete final code unit. The 64 KiB line limit still applies.
- The local survey found no UTF-16 rollouts and accepted rate-limit lines at most
  900 bytes. This is evidence about that snapshot, not a guarantee about all inputs.
- The existing line cap is a real regression: a newer valid line above the cap is
  skipped, allowing an older reading to win. Restoring exact behavior requires
  bounded streaming JSON validation and field extraction; merely increasing the
  cap passes the allocation problem to the existing materializing parser.
- Scanner buffers are bounded, but total allocation grows with the number of
  candidate lines parsed and rejected. A bounded individual parse is not a bound
  on the aggregate allocation for arbitrary input.

## Lifecycle mismatch

The frozen snapshot's newest eligible rollout was 165,327 bytes and returned a
valid reading. The 380,766,200-byte rollout ranked second, so `CodexIn` stopped
before reading it. The old standalone run therefore did not reproduce the large
read that produced the original memory baseline. The previous harness reports
total working set rather than private working set, truncates protocol snapshots,
and combines CPU time fields with bitwise OR rather than addition. Its CPU and
correctness data do not establish acceptance.

## Fresh checks

- `gofmt -l .`: no output; `go vet ./...`: pass; `go test ./... -count=1`: pass.
- Focused tail and Codex tests: pass, including UTF-16 endianness, separators,
  window boundaries, an incomplete final byte, and a 40 MiB UTF-16 fixture.
- Synthetic `BenchmarkCodexInLarge`: 1,530,749 ns/op, 360,722 B/op, 129 allocs/op
  on this run. This is not a controlled before/after latency comparison.
- Rebuilt parity drivers against the old tree and the current tree and ran both on
  the frozen 88-file, 853,919,418-byte snapshot at the same fixed time: 88/88
  readings matched. Per-file total allocation was 2,792.7 versus 30.6 MiB;
  maximum per-file heap in use was 1,201.5 versus 1.7 MiB. These are independent
  per-file calls and do not prove a production resident-memory improvement.
  The combined run selected the small file and allocated 2.1 versus 1.8 MiB.
  Raw report: `%TEMP%\aqwork\parity\out\utf16-restored-review\compare.txt`.

## Remaining acceptance work

Implement and review bounded oversized-record handling with legacy JSON semantics;
rerun parity and full tests; validate a measurement harness using a frozen home
that demonstrably selects the large file. Compare old and new service processes
with complete readings, latency, CPU, committed memory and private working set,
then perform the requested one-hour stability run. The production helper and
Bloom integration require separate live verification after explicit deployment
approval. No production process was restarted or modified during this work.
