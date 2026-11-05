# Complete agent quota details

The installed overview now shows every supported quota category retained by the
seven readers. Selected details grow to fit complete wrapped rows. The compact
rail and matte-black material remain. Each reset stays with its quota, and stale
readings retain their last good details alongside the latest error.

| Agent | Selected details |
| --- | --- |
| Claude | Reported plan, five-hour and weekly usage, and both reset times. Status-line snapshots retain their original observation time. |
| Kiro | Reported plan, credits used and covered allowance, and the source reset date. |
| Codex | Every reported window, its reset, and the source event time when present. |
| Cursor | Plan, total, Auto and API percentages, reported amounts and remaining allowance, on-demand status, plan breakdown, and billing reset. |
| Copilot | Plan, every finite or explicitly unlimited category, allowance and remaining values, reported usage, meaningful overage status, resets, and source observation. |
| Devin | Reported plan, daily and weekly usage, and each window's reset. |
| Antigravity | Every enabled model bucket and its own reset. Duplicate buckets remain distinct. |

Readers emit `quota.Detail` rows with a label, value, reset instant, or source date
text. `main.buildView` formats instants from one clock and copies the rows into
the widget. `widget/detail.go:selectedDetailLayout` owns wrapped lines, height,
and control rectangles behind `Render(View)`. Missing values remain absent.
Missing or unreadable Copilot quotas no longer become Unlimited. Existing
headline selection, five-hour pill timers, polling, deadlines, requests,
credential handling, and runtime dependencies remain.

The sanitized live Copilot response supplied `copilot_plan`, `entitlement`,
`quota_remaining`, and `credits_used`, which the older fixtures omitted. The
parser preserves those independent facts without reconciling conflicting source
counters. The live Cursor response also supplied remaining amounts, on-demand
status, and a labelled plan breakdown. These mappings have source-shaped
regression fixtures. Numeric amount fields retain their reported values without
an assumed currency conversion. No account identifiers, credentials, or raw
responses were saved.

Verification completed against the final code.

- `gofmt -l .` is empty. Temporary diagnostic programs were formatted too.
- `go vet ./...` and the executable build pass.
- `go test ./... -count=1` passes for all nine packages on the interactive
  Windows desktop, including the native screen-paint test.
- The installed executable's `--print` check exits 0 for all seven readers.
- A normalized live read retained 2 Claude rows, 1 Kiro row, 2 Codex rows,
  6 Cursor rows, 4 Copilot rows, 2 Devin rows, and 5 Antigravity rows.
- `--shot` generated 84 complete-detail fixtures across seven agents, full and
  narrow views, stale states, and 100%, 125%, 150%, and 200% scales.
- All seven installed selected-agent views were captured and inspected under
  `.impeccable/review/all-agent-details/after-*.png`. Every current row fits,
  including the final model row, with separate footer controls.

Independent scoped review found no material defects in layout, propagation,
stale handling, or the initial non-Copilot parser changes. Root reviewed the
later Copilot and Cursor source mappings and verified them against live quota
fields, literal regression expectations, and the complete suite. The comment
review's one internal narrative comment was removed. Public data-contract
comments remain. Initial design delegates failed at the account usage limit;
the two design alternatives and their comparison were reconstructed by root.
No independent design comparison is claimed.

The installed and workspace executables match SHA256
`98479103AE90A03C3E1A9D290B6193AD619EB73EF6862C160902717C11A21BD1`.
Existing icon and version resource exports compare byte-for-byte. Guarded
replacement saved rollback executables and settings under `.debug-launch`.
Restart preserved foreground focus, position, Compact, visibility, and the
Claude selection observed immediately afterward. The settings later changed to
Codex after the capture run; that valid selection was left untouched. The
machine-readable summary is
`.debug-launch/all-agent-details/verification-summary.json`.

The separate agent-click pin correction is complete, and its required design
approval was received. The earlier handler was byte-identical to the baseline;
the fix and verified native behavior are recorded in
[the pin-selection verification](2026-10-16-pin-selection.md). This supersedes
the earlier pending status and preliminary runtime observations.

The local trail is `.debug-launch/all-agent-details/decisions.tsv`. Source-shaped
fixtures and current native captures establish the supported seven-reader
cases. Arbitrarily oversized upstream labels or category lists have not been
verified against a scrolling or height-cap mechanism.

A fresh GPT-6 Astra trail review checked the active workspace transcript against
the closeout records and found no additional material flags. Its report is
`.debug-launch/all-agent-details/trail-review.md`. It confirms the pending pin
approval at that time; that pending status is now resolved by the pin-selection
verification above. The documented review limits remain. Only GPT-family models
were available
in this environment; no different-family review is claimed.
