# Codex release readiness follow-up

The committed base `eb211b0` fails `go test ./...` to compile: `main.go`
assigns `widget.Item.Help`, but the committed `widget.Item` has no such field.
Commit `39cded4` adds that field only; it does not include the unrelated
uncommitted widget redesign. The Codex optimization was cherry-picked unchanged
as `5067ef0`. Its `quota/` changes match `94feb48` exactly.

The integration test in `dadfced` sends Codex refresh requests through the
service handler using isolated rollout files and serializes the resulting
snapshots. For two successive values, percentage and reset time match a direct
`CodexIn` reading. Existing Go service tests cover hello, refresh, errors and
shutdown. These tests do not start Bloom or exercise its UI or supervisor with
a real child process.

On the clean integration checkout, `go build`, `go test ./... -count=1`,
`go vet ./...` and `gofmt -l .` passed. The latter used an LF checkout;
the host's global `core.autocrlf=true` otherwise flags CRLF files. Bloom's
`cargo test --offline --bin bloom quota_helper::tests` passed 9 tests, covering
message handling, refresh encoding, helper arguments and exit-state handling.
These are component tests, not an end-to-end Bloom/helper session. The Rust
build emitted an existing unnecessary-unsafe warning in `services.rs`.

Only the active Windows console session was available; no Windows VM tooling
or second desktop session was found. A second Bloom in the active session is
not isolated: its single-instance mutex and taskbar operations are shared.
Do not launch it there for this test. Actual notch presentation, automatic
refresh, Bloom-to-child lifecycle and taskbar behavior remain unverified.

The old A/B run's original handle criterion, `max - min <= 50`, failed. Its
post-start interpretation is not a pass against that original criterion. The
separate one-hour test used a predeclared first-five-minute baseline and met
that different criterion; roughly 90 handles disappeared during that run for
an unknown reason. Historical performance results and their limitations remain
in `2026-10-29-memory-baseline.md`.

The temporary rollback copies and their expected SHA-256 hashes are documented
in `2026-10-29-codex-rollback.md`. They must be copied to durable storage and
verified before any approved deployment. No deployment or rollback was made.

Recommendation: HOLD pending isolated real Bloom integration and a decision
on the original failed handle criterion.
