# Codex oversized records: legacy compatibility contract

Status: oversized-record streaming fallback implemented; validation evidence is
recorded separately from lifecycle and production acceptance.

The old reader decodes the complete file with `core.TextOf`, splits on CR and
LF, and examines lines containing the literal text `"rate_limits"`. It chooses
the last line accepted by `ParseCodexLine` in the first eligible ranked rollout.
The literal prefilter is independent of JSON key decoding: an escaped key alone
does not trigger parsing, even if it decodes to `rate_limits`.

`core.ParseJSON` validates the complete line, including ignored and overwritten
values. It rejects trailing data, invalid JSON numbers, bad escapes, lone escaped
surrogates and containers deeper than 64. Object keys are decoded, compared
case-sensitively, and the last duplicate replaces the previous value at every
level. A duplicate object does not merge with the previous object. UTF-8 decoding
is lossy; BOM-selected UTF-16LE/BE is decoded lossily, and an incomplete final
code unit is discarded. An incomplete JSON record is rejected, while a complete
record without a final newline can be accepted.

Only object-valued `payload.rate_limits`, and numeric values within object-valued
`primary` and `secondary`, contribute to a reading. Non-numeric, overflowing or
missing numeric fields act as absent; finite underflow is accepted. Conversion uses
`strconv.ParseFloat` with its exact float64 rounding. The timestamp is parsed after
Unicode whitespace trimming; an invalid timestamp defaults to the supplied clock.
The reading, details, windows, observed time and reset fields follow
`ParseCodexLine`; its separate `CodexPrimaryReset` pass can contribute a reset
detail even when primary has no usage and secondary supplies the reading.

The original 64 KiB tail cap silently discarded a valid newer record. It now
selects a streaming fallback; larger records are not skipped solely for size.
The standard `encoding/json.Decoder` cannot directly provide bounded parsing:
`Decode` buffers a whole value and `Token` calls `Decode` for individual strings
and numbers. Its acceptance behavior also differs from `core.ParseJSON`.

A bounded streaming validator retains a projection and reuses `ParseCodexLine`.
Numeric conversion retains the first 800 significant digits, the sticky bit and
the separate fast and fallback decimal-point positions, then uses `strconv` to
perform the final conversions. A token cap or naive truncation would change
observable behavior; neither is used.

The installed Go 1.27 conversion code has an additional compatibility constraint:
`internal/strconv/atof.go` limits exponent accumulation, and its decimal fallback
keeps 800 significant digits with a sticky truncation flag. A mathematically exact
normalizer is not automatically equivalent to this implementation for extreme
mantissa/exponent cancellation. The oversized numeric oracle test includes a
rounding midpoint with a nonzero digit beyond 70,000 zeroes; retaining a short
prefix alone loses the distinction.

The converter mirrors implementation details of the installed Go 1.27 toolchain.
It must be reassessed against `strconv` if that implementation changes. Numeric
differential tests cover 5,005 generated and targeted examples, not every possible
input. A frozen 88-file parity run matched all compared fields; this snapshot is
not exhaustive proof of compatibility. The independent review's read-error and
allocation findings were fixed and covered by focused tests. Process lifecycle,
one-hour stability, and production behavior remain outside this evidence.

Verification on the final production code: `go test ./... -count=1`,
`go vet ./...`, and `gofmt -l .` passed. The frozen-snapshot parity harness
compared 88 files and reported 88 matching complete readings, with 2,792.7 MiB
versus 30.7 MiB total per-file allocations and 1,201.6 MiB versus 1.5 MiB
maximum sampled per-file heap in use. Raw results are in
`%TEMP%\aqwork\parity\out\oversized-stream-verified\compare.txt`.
These are per-read results, not resident service-memory measurements. Three
synthetic benchmark runs took 1.20–1.26 ms/op and 360,350–361,145 B/op.
One intervening full-suite run failed the native window hit test
`TestFlushPresentsOpaquePixels` because another window was hit. Three isolated
reruns and a subsequent complete suite passed. The failure remains an observed
native-test instability; the Codex and quota tests passed in that run.
