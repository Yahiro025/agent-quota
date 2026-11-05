# Codex weekly quota visibility

The supplied screenshot's 68% was the five-hour window, as its `5h` caption
indicated. A read-only Codex probe returned structured windows for both five-hour
and weekly usage (71% and 37% at observation), plus the five-hour reset instant.
The probe emitted only these parsed quota fields, without raw session logs.
`main.go:buildView` preserved both windows. The omission occurred in
`widget/seam.go:seamDetail`: whenever a reset existed, it returned the five-hour
window alone instead of the complete window metrics.

The footer now retains both percentages and explicitly qualifies the five-hour
reset. When the combined text exceeds the available width, the complete reset
moves below both percentages within the existing footer height. The shared path
also fixes Claude's weekly detail. Reader parsing, headline maximum-window
selection, reset calculations, credentials, polling and dependencies are unchanged.

Before the implementation changed, the regression reproduced hidden weekly usage
in full and narrow overviews at 100%, 125%, 150% and 200% DPI. It now passes and
also verifies that countdown and clock minute changes remain visible. The existing
reset-meaning test now requires both reported percentages while retaining an
explicit `5h resets` qualification. Frozen `--shot` exports include full and
narrow Codex states at all four scales under
`.impeccable/review/seam/codex-windows/`; the full 100%/150%/200% and narrow
125%/150% images were visually inspected.

Verification: `gofmt -l .` is empty, `go vet ./...` and the build pass, and
`go test ./... -count=1` passes for all nine packages on the interactive Windows
desktop, including the native screen-paint test. `go run . --print` with normal
network access exits 0 with all seven readers successful and both Codex windows.
The network-restricted sandbox's earlier live-reader attempt could not reach
remote services; it was not used as reader-health evidence.

The installed executable at `%LOCALAPPDATA%\Programs\AgentQuota\agentquota.exe`
and the workspace executable match the prepared build. Existing icon/version
resources were preserved and their exports compare byte-for-byte. Guarded
replacement saved rollback copies under `.debug-launch`, preserved foreground
focus and retained position, Compact, selection and agent visibility preferences.
The installed copy was revealed without activation. A widget-only capture at
`.impeccable/review/seam/live-codex-windows-fix.png` shows Codex `5h 75%`,
`Week 38%`, and `5h resets in 25m` together. The deployed SHA256 is
`1F4847E55BE80539AC05C9AE59C37B1EE9255E510F3EF6B2AF20D9BAEAC3ED0A`.
