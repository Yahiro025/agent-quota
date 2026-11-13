# Agent selection preserves pin state

The approved fix is complete. `main.go` now sets `a.open = true` when an agent
is selected, without changing the pin state. `pin_selection_test.go` covers the
selection behavior. The worker recorded 14 failing unpinned cases before the
fix; the test passed afterward in 1.556 seconds.

Root verified `go test ./... -count=1` across all nine packages, `gofmt -l .`,
`go vet ./...`, and the executable build. The installed executable matches
SHA256 `EBD2265BF31F5BB18E5D8EA30D6C5388A03CC197820A05ADB4DD505090A2EFF1`;
its original icon and version resources are preserved.

Native runtime evidence is in
`.debug-launch/pin-selection/runtime-result.json`. All seven clicked agent IDs
matched their expected selections. Each unpinned selection closed to 68 pixels
after pointer leave. Pinned Claude and Kiro remained expanded at 279 and 255
pixels; explicitly unpinning closed to 68 pixels. Foreground, selection,
Compact, and visibility were preserved. The final state was unpinned and
closed.
