# Antigravity usage failure and recovery

The installed `agy` 1.3.5 launched normally. Live probes reproduced both a
25-second stall without quota output and an early CLI failure. The CLI's own
failed-run log identified the latter as backend `503 UNAVAILABLE`. A subsequent
run from the original project directory succeeded, so the initial working-
directory hypothesis was rejected; no directory or CLI configuration changed.
Diagnostics emitted timings, response sizes and allowlisted error metadata,
without saving raw responses or credentials.

`quota/antigravity.go` now retries a recognized structured 503 once, after 250 ms,
when enough of the original 25-second usage watchdog remains. The second CLI
print timeout is shortened to the remaining budget. The five-second version
check and the first attempt's 20-second print timeout remain. Both processes run
sequentially inside one reader invocation. Persistent 503s receive a service-
unavailable explanation. Authentication failures, model turns, unrecognized
responses and watchdog timeouts are not retried.

Regression tests exercise recovery, the two-attempt ceiling, a hung retry's
shared deadline, and precedence of authentication/model-turn rejection. The
existing output-size, CLI version, process-tree timeout and quota parsing tests
also pass. No dependency, credential, polling schedule or UI changes were made.

Verification: `gofmt -l .` is empty, `go vet ./...` and the build pass, and
uncached `go test ./... -count=1` passes on the interactive Windows desktop.
`go run . --print` exits 0 with successful readings for all seven agents;
Antigravity reports 10% and three model groups.

The running copy was the older installation under
`%LOCALAPPDATA%\Programs\AgentQuota\agentquota.exe`. That copy and the workspace
executable now contain the verified fix. Its existing icon/version resources
were extracted and preserved with the already-installed `go-winres` build tool;
resource exports compare byte-for-byte. The installed build remains version
0.1.0 and unsigned. No runtime dependency was added.

Guarded replacement saved rollback copies of the installed executable and app
settings under `.debug-launch`. The legacy settings migration retained the
existing valid position and visibility preferences. Restart preserved foreground
focus. A live widget-only capture at
`.impeccable/review/seam/live-antigravity-fix.png` confirms all seven readings,
including Antigravity at 10%. The deployed executable SHA256 is
`336538DB5DC41B720349AA17EEAF66FD0606561F513DD9DFB525F403214C7419`.

The CLI's read-only print-mode quota command is documented in the
[official changelog](https://github.com/google-antigravity/antigravity-cli/blob/main/CHANGELOG.md#1111).
Live successful probe envelopes also reported zero model turns and no
conversation. The guarded model-turn latch remains in place.
