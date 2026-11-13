# Agent Quota RAM optimization handoff

## Objective and boundaries

Continue the Codex rollout reader memory optimization in `%USERPROFILE%\Documents\PROJECT\agent-quota`. The user wants exact existing readings, bounded memory, no performance regression, comparable lifecycle measurements and a one-hour stability test, followed by a justified MERGE or HOLD recommendation. Delegate isolated tasks when useful; an independent critical reviewer is preferred, and Claude API availability must not be assumed.

Do not restart or modify the running production helper, launch the floating widget, merge, or deploy without the user's approval. Preserve the existing implementation and the many unrelated dirty and untracked files. Read the repository's `AGENTS.md` before work; the Bloom helper and notch constraints apply.

## Current state

**HOLD: the current backward tail reader skips valid rate-limit lines larger than 64 KiB**, so it can return an older reading. This violates the requested compatibility. The old parser materializes JSON and cannot simply be called on arbitrary-size input without losing the memory bound. A bounded streaming validation and selective extraction path must preserve `core.ParseJSON` behavior, including duplicate keys, strict validation and depth, numeric semantics, the literal `"rate_limits"` filter, and `ParseCodexLine`'s behavior. The UTF-16 path also currently retains the line-size cap. Individual scanner buffers are bounded, but aggregate allocations can grow with the number of rejected candidate lines.

The preceding agent restored UTF-16LE/BE BOM reading for bounded lines and added focused tests. Independent static review found no further critical UTF-16 defect; it flagged test gaps that were subsequently addressed. The final malformed-code-unit test was run in the focused suite after the last full-suite run.

## Evidence and code

- Current implementation and tests: `quota/read.go`, `quota/codex_tail.go`, `quota/codex_tail_test.go`. The latter two are untracked; do not overlook them when inspecting the diff.
- Takeover findings, current results, benchmark limitations and acceptance gaps: `docs/verification/2026-10-29-codex-tail-takeover.md`.
- Original investigation and baseline: `docs/verification/2026-10-29-memory-baseline.md`.
- Prior plan and pre-implementation review: `docs/superpowers/plans/2026-10-29-codex-tail-reader.md`. Its deliberate long-line and UTF-16 deviations are not authorization to violate the user's current compatibility requirement.
- Most recent frozen-snapshot parity report: `%TEMP%\aqwork\parity\out\utf16-restored-review\compare.txt`; parity scripts and frozen input are under `%TEMP%\aqwork\parity` and `%TEMP%\aqwork\snapshot`. These temporary artifacts may disappear; verify before using.
- Old and new standalone A/B files and harness are under `%TEMP%\aqwork\ab` and `%TEMP%\aqprobe\harness`. The existing harness is unsuitable for final acceptance for the reasons in the takeover report.

## Next steps

1. Design, implement, and critically review a bounded oversized-record path that preserves the legacy parser's acceptance and output. Add regression tests for valid long lines, long malformed records, duplicates, depth, encodings, and bounded memory. Preserve the existing fast path where appropriate.
2. Rerun focused and full tests, formatting, vet, frozen-snapshot parity, and allocation/latency benchmarks against the final code. Report actual results and limitations.
3. Build a verified comparable lifecycle harness: ensure identical frozen input and demonstrate the large rollout is selected, identify binaries and environment, capture complete correctness results and latency, and measure commit, **private** working set and CPU correctly. The previous 10-minute runs selected a small successful rollout first and did not reproduce the original workload.
4. Once the workload and instrumentation are verified, perform the requested one-hour stability test and report results. Live production memory and Bloom notch behavior remain unverified and require a separately approved deployment; do not equate isolated process results with production verification.
5. Obtain an independent review of the final diff and evidence, resolve confirmed issues, and provide a justified MERGE or HOLD recommendation. Do not merge or deploy without approval.

## Suggested skills

- Call the Skill tool for `diagnosing-bugs` when investigating remaining parser and lifecycle discrepancies.
- Call the Skill tool for `benchmark-checklist` when designing and validating comparable measurements.
- Call the Skill tool for `code-review` for the final critical review.
