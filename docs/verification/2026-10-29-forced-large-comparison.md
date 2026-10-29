# Forced-large Codex comparison

Status: SUPERSEDED for the oversized-line defect, kept as history. The oversized-line
compatibility defect named below is fixed by the streaming parser in
`quota/codex_stream.go`. This remains a per-read investigation, not a service lifecycle
test; the lifecycle comparison is in `2026-10-29-memory-baseline.md`.

Original status: HOLD. This is a per-read investigation, not a service lifecycle or
production-memory acceptance test. The oversized-line compatibility defect is
still present and no parser fix is claimed here.

## Fixture and method

The frozen snapshot's largest rollout was hard-linked into an isolated sessions
directory containing exactly one file. Its length is 380,766,200 bytes and its
SHA-256 is `F88C1954A07D4BE5622BECD9C21E4C9E312FD2D8C61FA4966EC624BA969652B9`.
The existing parity harness built separate drivers against `%TEMP%\aqbase` and the
current checkout, ran sequentially with the same fixed clock and input, and called
`CodexIn`. Both returned a successful reading, so the only file was selected.
All recorded reading fields and their hashes matched. The source and fixture are
hard links: the fixture must remain read-only. The harness checks file names and
sizes, not a before/after content hash; the hash above was taken after the run.

Reproduce while the frozen snapshot and parity tooling remain available:

```powershell
$source = Get-ChildItem "$env:TEMP\aqwork\snapshot\sessions" -Recurse -File -Filter 'rollout-*.jsonl' | Sort-Object Length -Descending | Select-Object -First 1
$sessions = "$env:TEMP\aqwork\large-fixture\sessions"
New-Item -ItemType Directory -Force $sessions | Out-Null
New-Item -ItemType HardLink -Path (Join-Path $sessions $source.Name) -Target $source.FullName
powershell -NoProfile -ExecutionPolicy Bypass -File "$env:TEMP\aqwork\parity\run.ps1" -Sessions $sessions -RunName forced-large-takeover
```

Use a fresh empty fixture directory; do not create or modify files through an
existing hard link. Check the source hash before and after an acceptance run.

## Observations

| Metric | Legacy | Tail reader |
| --- | ---: | ---: |
| Per-file allocation | 1,215.9 MiB | 0.4 MiB |
| Sampled peak heap in use | 1,201.5 MiB | 1.4 MiB |
| Per-file call latency, one sample | 725.8 ms | 20.1 ms |
| Driver peak total working set | 1,258.6 MiB | 14.9 MiB |
| Driver peak commit | 1,257.9 MiB | 48.7 MiB |

The heap sampler runs at 1 ms and may miss shorter peaks. Driver peaks include
harness work and are not private working set or service measurements. The two
latencies are single observations, not a performance distribution. The comparison
also performs a second combined call on the same one-file fixture. Raw reports and
binary source hashes are in `%TEMP%\aqwork\parity\out\forced-large-takeover`.

Current checkout: `go test ./... -count=1`, `go vet ./...`, and `gofmt -l .`
passed. These results do not prove oversized-record compatibility. The earlier
88-file parity run and its limitations are in the takeover report.

## Remaining work

Implement and independently review a bounded streaming validator and projection
for oversized UTF-8 and UTF-16 records. Preserve full-document validation, decoded
duplicate-key semantics, the raw literal filter, and `CodexPrimaryReset` behavior.
Long numeric values need exact `strconv.ParseFloat`-compatible conversion without
an unbounded token buffer. Unicode-trimmed timestamps can be bounded because
`ParseStamp` rejects trimmed strings over 42 bytes. Add regression and parity
tests, then build and validate a lifecycle harness that captures complete readings,
private working set, commit, CPU and latency with identical input and schedule.
Only after that workload is verified should the one-hour run be used for acceptance.
