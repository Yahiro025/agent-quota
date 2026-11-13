package quota

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"agent-quota/internal/core"
)

// The backward tail scan behind CodexIn must return what the old whole-file loop returned for
// UTF-8 rollouts, at every window size, and must stay bounded in memory and I/O however large
// the rollout is. Equivalence is checked against oracleLast, a copy of that loop. The bounds are
// checked on tailVirtual files, which are far too large to hold in memory.
//
// No test here uses t.Parallel: the allocation checks read process-wide counters.

var tailNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

// tailBOM is the UTF-8 byte order mark. The scan drops it from the first line only.
const tailBOM = "\xef\xbb\xbf"

// tailWindows are the window sizes the equivalence checks run with. 1, 2, 3 and 7 put a window
// edge inside nearly every line; codexTailWindow is the size the scan ships with.
var tailWindows = []int{1, 7, 64, 4096, codexTailWindow}

// tailEnds are the terminators. The old loop split on \n, then each piece on \r.
var tailEnds = []string{"\n", "\r\n", "\r"}

const tailFiller = "an ordinary event line with no limits in it"

// tailAccepted is a line ParseCodexLine accepts: one primary window whose reset is two hours
// after tailNow. It has the shape of codexLine in quota_upstream_test.go.
func tailAccepted(used float64) string {
	return fmt.Sprintf(`{"timestamp":"2026-10-29T11:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":%v,"window_minutes":300,"resets_at":%d}}}}`,
		used, tailNow.Add(2*time.Hour).Unix())
}

// tailMinimal is the shortest accepted line, 58 bytes. It fits under the 64-byte cap the
// over-cap tests use.
func tailMinimal(used float64) string {
	return fmt.Sprintf(`{"payload":{"rate_limits":{"primary":{"used_percent":%v}}}}`, used)
}

// tailPadOpen and tailPadClose wrap a pad string, so tailPadded can lengthen an accepted
// line to any size from 67 bytes up.
const (
	tailPadOpen  = `{"payload":{"rate_limits":{"primary":{"used_percent":5}}},"pad":"`
	tailPadClose = `"}`
)

// tailPadded is an accepted line of exactly n bytes, n >= 67.
func tailPadded(n int) string {
	base := len(tailPadOpen) + len(tailPadClose)
	if n < base {
		panic(fmt.Sprintf("tailPadded: %d is under the %d-byte minimum", n, base))
	}
	return tailPadOpen + strings.Repeat("x", n-base) + tailPadClose
}

// tailEscapedKey is a valid accepted line whose key is written with a \u escape for "r". JSON
// reads it as "rate_limits", but the quoted needle is not among its bytes. The escape is put
// together from an interpreted string, so it cannot be flattened to a plain r on the way in.
func tailEscapedKey() string {
	return strings.Replace(`{"payload":{"@ate_limits":{"primary":{"used_percent":5}}}}`, "@", "\\u0072", 1)
}

// tailNoted is an accepted line whose first key is a free-text note. The note is put in
// unchanged, so callers can place invalid UTF-8 inside an accepted line.
func tailNoted(note string) string {
	return `{"note":"` + note + `",` + tailAccepted(7)[1:]
}

// tailRejected hold the needle, but ParseCodexLine refuses every one. A scan that stops at the
// first needle line, or that accepts a refused line, fails on these.
var tailRejected = []string{
	`{"payload":{"rate_limits":null}}`,
	`{"payload":{"rate_limits":{"primary":{"used_percent":"x"}}}}`,
	`{"payload":{"rate_limits":{"primary":{"used_percent":5,"resets_at":1e20}}}}`,
	`{"payload":{"rate_limits":{"primary":{"used_percent":4`,
	`{"rate_limits":{"primary":{"used_percent":5}}}`,
	`"rate_limits"`,
}

// oracleLast is the old CodexIn loop on already-decoded text: split on \n, split each piece
// on \r, and keep the last line that holds the quoted needle and that ParseCodexLine accepts.
func oracleLast(text string, now time.Time) (Reading, bool) {
	var last *Reading
	for _, l := range strings.Split(text, "\n") {
		for _, line := range strings.Split(l, "\r") {
			if !strings.Contains(line, `"rate_limits"`) {
				continue
			}
			if q, ok := ParseCodexLine(line, now); ok {
				last = &q
			}
		}
	}
	if last != nil {
		return *last, true
	}
	return Reading{}, false
}

// oracleBytes is the old loop on a UTF-8 file: the byte order mark is dropped, each line is
// decoded with Lossy, then oracleLast runs. The old UTF-16 decoding is gone, so callers pass
// UTF-8 only.
func oracleBytes(raw []byte, now time.Time) (Reading, bool) {
	return oracleLast(core.Lossy(bytes.TrimPrefix(raw, []byte(tailBOM))), now)
}

// oracleCapped retains its old test-helper signature, but the cap now selects
// a parsing strategy and does not change the legacy result.
func oracleCapped(raw []byte, now time.Time, lineCap int) (Reading, bool) {
	var last *Reading
	start := 0
	for i := 0; i <= len(raw); i++ {
		if i < len(raw) && raw[i] != '\n' && raw[i] != '\r' {
			continue
		}
		piece := raw[start:i]
		_ = lineCap
		if start == 0 {
			piece = bytes.TrimPrefix(piece, []byte(tailBOM))
		}
		s := core.Lossy(piece)
		start = i + 1
		if !strings.Contains(s, `"rate_limits"`) {
			continue
		}
		if q, ok := ParseCodexLine(s, now); ok {
			last = &q
		}
	}
	if last != nil {
		return *last, true
	}
	return Reading{}, false
}

// tailRun scans raw as a bytes.Reader.
func tailRun(raw []byte, window, lineCap int) (Reading, bool, error) {
	return lastRateLimits(bytes.NewReader(raw), int64(len(raw)), tailNow, window, lineCap)
}

// tailShow is a short description of a result for failure messages.
func tailShow(r Reading, ok bool) string {
	if r.Used == nil {
		return fmt.Sprintf("ok=%v, no reading", ok)
	}
	return fmt.Sprintf("ok=%v, used=%v, detail=%q", ok, *r.Used, r.Detail)
}

// tailCheck asserts one scan result. When an accepted line is expected it must equal want
// exactly; when none is expected only ok=false and a nil error are required.
func tailCheck(t *testing.T, what string, got Reading, ok bool, err error, want Reading, wantOK bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", what, err)
	}
	if ok != wantOK {
		t.Fatalf("%s: got %s, want ok=%v", what, tailShow(got, ok), wantOK)
	}
	if wantOK && !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %s, want %s", what, tailShow(got, ok), tailShow(want, wantOK))
	}
}

// tailExpectLineIn checks, at each window size in windows and against the oracle, that the last
// accepted line of raw is line. The expected Reading is ParseCodexLine's own answer for line, so
// the test does not depend on the oracle alone.
func tailExpectLineIn(t *testing.T, what string, raw []byte, lineCap int, line string, windows []int) {
	t.Helper()
	want, ok := ParseCodexLine(string(core.Lossy(bytes.TrimPrefix([]byte(line), []byte(tailBOM)))), tailNow)
	if !ok {
		t.Fatalf("%s: fixture line is not accepted", what)
	}
	if got, gok := oracleCapped(raw, tailNow, lineCap); !gok || !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: the oracle does not find the fixture line", what)
	}
	for _, w := range windows {
		got, gok, err := tailRun(raw, w, lineCap)
		tailCheck(t, fmt.Sprintf("%s, window %d", what, w), got, gok, err, want, true)
	}
}

// tailExpectNoneIn checks that no window size in windows finds an accepted line in raw.
func tailExpectNoneIn(t *testing.T, what string, raw []byte, lineCap int, windows []int) {
	t.Helper()
	if _, ok := oracleCapped(raw, tailNow, lineCap); ok {
		t.Fatalf("%s: the oracle finds a line", what)
	}
	for _, w := range windows {
		got, ok, err := tailRun(raw, w, lineCap)
		tailCheck(t, fmt.Sprintf("%s, window %d", what, w), got, ok, err, Reading{}, false)
	}
}

// tailExpectLine is tailExpectLineIn at every window size in tailWindows.
func tailExpectLine(t *testing.T, what string, raw []byte, lineCap int, line string) {
	t.Helper()
	tailExpectLineIn(t, what, raw, lineCap, line, tailWindows)
}

// tailExpectNone is tailExpectNoneIn at every window size in tailWindows.
func tailExpectNone(t *testing.T, what string, raw []byte, lineCap int) {
	t.Helper()
	tailExpectNoneIn(t, what, raw, lineCap, tailWindows)
}

func TestTailLastAcceptedLineWins(t *testing.T) {
	raw := tailAccepted(10) + "\n" + tailFiller + "\n" +
		tailAccepted(20) + "\n" + tailFiller + "\n" +
		tailAccepted(30) + "\n" + tailFiller + "\n"
	tailExpectLine(t, "three accepted lines", []byte(raw), codexLineCap, tailAccepted(30))
}

func TestTailRejectedNewestFallsBack(t *testing.T) {
	var b strings.Builder
	b.WriteString(tailAccepted(10) + "\n")
	b.WriteString(tailAccepted(20) + "\n")
	// Many refused needle lines after the accepted one: the scan has to step over all of them,
	// across several windows, to reach it.
	for i := 0; i < 100; i++ {
		b.WriteString(tailRejected[i%len(tailRejected)] + "\n")
	}
	tailExpectLine(t, "refused needle lines after the accepted one", []byte(b.String()), codexLineCap, tailAccepted(20))
}

func TestTailTerminators(t *testing.T) {
	lines := []string{tailAccepted(10), tailFiller, tailMinimal(20), tailRejected[0], tailFiller, tailAccepted(30)}
	styles := []struct {
		name string
		end  func(i int) string
	}{
		{"LF", func(int) string { return "\n" }},
		{"CRLF", func(int) string { return "\r\n" }},
		{"CR", func(int) string { return "\r" }},
		{"mixed", func(i int) string { return tailEnds[i%len(tailEnds)] }},
	}
	for _, s := range styles {
		var b strings.Builder
		for i, l := range lines {
			b.WriteString(l)
			b.WriteString(s.end(i))
		}
		tailExpectLine(t, s.name+" terminators", []byte(b.String()), codexLineCap, tailAccepted(30))
	}
}

func TestTailPartialLastLine(t *testing.T) {
	// A file being written can end mid-line. The fragment is a line like any other: refused
	// when it is truncated JSON, so the reading comes from the line before it.
	tailExpectLine(t, "truncated last line", []byte(tailAccepted(10)+"\n"+tailRejected[3]),
		codexLineCap, tailAccepted(10))
	// A complete accepted line with no terminator is read.
	tailExpectLine(t, "accepted last line without a terminator", []byte(tailAccepted(10)+"\n"+tailAccepted(20)),
		codexLineCap, tailAccepted(20))
}

func TestTailFirstLineIsEvaluated(t *testing.T) {
	// The only line of the file sits at offset 0. A scan that drops the head of the file
	// unless a terminator was seen before it would miss this.
	tailExpectLine(t, "only line, terminated", []byte(tailAccepted(10)+"\n"), codexLineCap, tailAccepted(10))
	tailExpectLine(t, "only line, unterminated", []byte(tailAccepted(10)), codexLineCap, tailAccepted(10))
	tailExpectLine(t, "only line, CR terminated", []byte(tailAccepted(10)+"\r"), codexLineCap, tailAccepted(10))
}

// tailRandomFile is a seeded file of mixed content: accepted lines of several sizes, lines
// with invalid UTF-8 inside, refused needle lines, filler, long filler, the three
// terminators, an optional UTF-8 BOM and an optional unterminated last line.
func tailRandomFile(rng *rand.Rand) []byte {
	var b []byte
	if rng.Intn(8) == 0 {
		b = append(b, tailBOM...)
	}
	lines := 1 + rng.Intn(14)
	for i := 0; i < lines; i++ {
		var line string
		switch rng.Intn(9) {
		case 0, 1:
			line = tailAccepted(float64(rng.Intn(1000)) / 10)
		case 2:
			line = tailMinimal(float64(rng.Intn(100)))
		case 3:
			// Around the 64 and 70 byte caps the random caps use.
			line = tailPadded(67 + rng.Intn(12))
		case 4:
			line = tailNoted(tailNoise(rng))
		case 5:
			line = tailRejected[rng.Intn(len(tailRejected))]
		case 6:
			line = tailText(rng, rng.Intn(40))
		default:
			line = tailText(rng, 200+rng.Intn(600))
		}
		b = append(b, line...)
		if i == lines-1 && rng.Intn(4) == 0 {
			break
		}
		b = append(b, tailEnds[rng.Intn(len(tailEnds))]...)
	}
	return b
}

// tailNoise is random text that may be invalid UTF-8. It has no control characters, quotes or
// backslashes, so the JSON string around it stays valid.
func tailNoise(rng *rand.Rand) string {
	b := make([]byte, 1+rng.Intn(20))
	for i := range b {
		c := byte(0x20 + rng.Intn(0x5f)) // printable ASCII
		if rng.Intn(3) == 0 {
			c = byte(0x80 + rng.Intn(0x80)) // high bytes: mostly invalid UTF-8
		}
		if c == '"' || c == '\\' {
			c = 'q'
		}
		b[i] = c
	}
	return string(b)
}

// tailText is n random letters: no terminators and no needle.
func tailText(rng *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + rng.Intn(26))
	}
	return string(b)
}

func TestTailWindowSweep(t *testing.T) {
	// Every window size from 1 to 300 on one file whose lines have many lengths and endings,
	// so line boundaries land on every offset of some window.
	parts := []string{
		tailAccepted(5), strings.Repeat("f", 210), tailRejected[0], tailAccepted(12.5), "",
		strings.Repeat("z", 333), tailRejected[1], tailAccepted(40), strings.Repeat("y", 77), tailRejected[3],
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		b.WriteString(tailEnds[i%len(tailEnds)])
	}
	raw := []byte(b.String())
	want, ok := ParseCodexLine(tailAccepted(40), tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	if got, gok := oracleCapped(raw, tailNow, codexLineCap); !gok || !reflect.DeepEqual(got, want) {
		t.Fatal("the oracle does not find the fixture line")
	}
	for w := 1; w <= 300; w++ {
		got, gok, err := tailRun(raw, w, codexLineCap)
		tailCheck(t, fmt.Sprintf("fixed file, window %d", w), got, gok, err, want, true)
	}

	// Seeded random files, compared with the oracle at every window size.
	rng := rand.New(rand.NewSource(20261029))
	found := 0
	for i := 0; i < 200; i++ {
		raw := tailRandomFile(rng)
		want, wantOK := oracleBytes(raw, tailNow)
		if got, gok := oracleCapped(raw, tailNow, codexLineCap); gok != wantOK || (gok && !reflect.DeepEqual(got, want)) {
			t.Fatalf("random file %d: the oracle's split disagrees with oracleBytes", i)
		}
		if wantOK {
			found++
		}
		for _, w := range tailWindows {
			got, gok, err := tailRun(raw, w, codexLineCap)
			tailCheck(t, fmt.Sprintf("random file %d (%d bytes), window %d", i, len(raw), w), got, gok, err, want, wantOK)
		}
	}
	if found < 50 {
		t.Fatalf("only %d of 200 random files have a limits line; the generator is too weak", found)
	}

	// The same files with a fast-path cap that many lines exceed.
	caps := []int{64, 70, 300}
	found = 0
	for i := 0; i < 200; i++ {
		raw := tailRandomFile(rng)
		lineCap := caps[rng.Intn(len(caps))]
		want, wantOK := oracleCapped(raw, tailNow, lineCap)
		if wantOK {
			found++
		}
		for _, w := range tailWindows {
			got, gok, err := tailRun(raw, w, lineCap)
			tailCheck(t, fmt.Sprintf("capped random file %d (%d bytes), cap %d, window %d", i, len(raw), lineCap, w), got, gok, err, want, wantOK)
		}
	}
	if found < 20 {
		t.Fatalf("only %d of 200 capped random files have a limits line; the generator is too weak", found)
	}
}

// Long lines must agree with the uncapped legacy oracle across window sizes.
func TestTailLongLinesUseLegacyOracle(t *testing.T) {
	cases := []string{tailMinimal(10) + "\n" + tailPadded(100<<10), tailPadded(100<<10) + "\n" + tailMinimal(20), tailMinimal(10) + "\n" + strings.Repeat("x", 100<<10)}
	for _, raw := range cases {
		want, wok := oracleBytes([]byte(raw), tailNow)
		for _, w := range []int{64, 4096, codexTailWindow} {
			got, ok, err := tailRun([]byte(raw), w, 64)
			tailCheck(t, "long line", got, ok, err, want, wok)
		}
	}
}
func TestTailNoLimits(t *testing.T) {
	tailExpectNone(t, "empty file", nil, codexLineCap)
	tailExpectNone(t, "terminators only", []byte("\n\r\n\r\r\n"), codexLineCap)
	var b strings.Builder
	for _, r := range tailRejected {
		b.WriteString(r + "\n")
	}
	tailExpectNone(t, "needle only in refused lines", []byte(b.String()), codexLineCap)

	// The needle is the quoted bytes, as the old loop had it. The escaped-key line is valid JSON
	// that ParseCodexLine accepts, but without the literal needle the old loop never looked at it.
	tailExpectNone(t, "escaped key has no literal needle", []byte(tailEscapedKey()+"\n"), codexLineCap)
}

func TestTailBOMAndInvalidUTF8(t *testing.T) {
	const bom = tailBOM
	tailExpectLine(t, "BOM before the first line", []byte(bom+tailAccepted(10)), codexLineCap, tailAccepted(10))
	tailExpectLine(t, "BOM before the first of two lines", []byte(bom+tailAccepted(10)+"\n"+tailAccepted(20)+"\n"),
		codexLineCap, tailAccepted(20))

	// A BOM is stripped only at offset 0. Mid-file it stays, and ParseCodexLine refuses the line.
	mid := bom + tailMinimal(30)
	if _, ok := ParseCodexLine(mid, tailNow); ok {
		t.Fatal("fixture: a BOM-prefixed line should be refused")
	}
	tailExpectLine(t, "BOM mid-file is not stripped", []byte(tailMinimal(10)+"\n"+mid+"\n"), codexLineCap, tailMinimal(10))

	// Invalid UTF-8 inside an accepted line. The old loop decodes each line on its own; the tail
	// scan must give the same reading.
	bad := tailNoted("\xff\xfe\xc3(\xe2\x82 ok")
	if _, ok := ParseCodexLine(core.Lossy([]byte(bad)), tailNow); !ok {
		t.Fatal("fixture: the invalid-UTF-8 line should be accepted once decoded")
	}
	tailExpectLine(t, "invalid UTF-8 inside an accepted line", []byte(tailAccepted(10)+"\n"+bad+"\n"), codexLineCap, bad)
	tailExpectLine(t, "invalid UTF-8 on the first line, after a BOM", []byte(bom+bad), codexLineCap, bad)

	// A BOM is stripped on the first line even when it crosses the fast-path cap.
	acc := tailAccepted(10)
	tailExpectLine(t, "BOM over the fast-path cap", []byte(bom+acc+"\n"), len(acc)+len(bom)-1, acc)
	tailExpectLine(t, "BOM counts toward the cap (exactly)", []byte(bom+acc+"\n"), len(acc)+len(bom), acc)
}

func TestTailHonoursSize(t *testing.T) {
	// size is the file as it was when it was measured. Bytes past it are not part of the file.
	a1, a2 := tailAccepted(10), tailAccepted(20)
	raw := []byte(a1 + "\n" + a2 + "\n")
	want1, ok := ParseCodexLine(a1, tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	want2, ok := ParseCodexLine(a2, tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	for _, w := range tailWindows {
		got, gok, err := lastRateLimits(bytes.NewReader(raw), int64(len(a1)+1), tailNow, w, codexLineCap)
		tailCheck(t, fmt.Sprintf("size ends after the first terminator, window %d", w), got, gok, err, want1, true)

		cut := int64(len(a1) + 1 + len(a2)/2)
		got, gok, err = lastRateLimits(bytes.NewReader(raw), cut, tailNow, w, codexLineCap)
		tailCheck(t, fmt.Sprintf("size cuts the second line in half, window %d", w), got, gok, err, want1, true)

		got, gok, err = lastRateLimits(bytes.NewReader(raw), int64(len(raw)), tailNow, w, codexLineCap)
		tailCheck(t, fmt.Sprintf("full size, window %d", w), got, gok, err, want2, true)
	}
}

// tailPiece is a run of a virtual file: literal bytes, or n generated bytes. Generated bytes
// are filler lines (the default), a run with no terminator (solid), or one filler line of n
// bytes whose last byte is a terminator (oneLine).
type tailPiece struct {
	lit     []byte
	n       int64
	solid   bool
	oneLine bool
}

func (p tailPiece) length() int64 {
	if p.lit != nil {
		return int64(len(p.lit))
	}
	return p.n
}

// tailPattern is one filler line: 99 bytes of x and a terminator. A line boundary falls at
// every global offset that is 99 mod 100.
var tailPattern = func() [100]byte {
	var p [100]byte
	for i := range p {
		p[i] = 'x'
	}
	p[99] = '\n'
	return p
}()

// tailSolid is a block of x with no terminator, copied from to fill solid runs.
var tailSolid = bytes.Repeat([]byte{'x'}, 4096)

var tailErrRefused = errors.New("tail test: a read was larger than the window")

// tailVirtual is an io.ReaderAt over pieces. It writes bytes into the caller's slice and keeps
// no copy, so a 1 GiB file costs nothing to hold. It records the bytes asked for in total, the
// largest single request, and whether a request exceeded maxReq, which is refused.
type tailVirtual struct {
	pieces  []tailPiece
	size    int64
	maxReq  int
	asked   int64
	biggest int
	refused bool
}

func tailNewVirtual(maxReq int, pieces ...tailPiece) *tailVirtual {
	v := &tailVirtual{pieces: pieces, maxReq: maxReq}
	for _, p := range pieces {
		v.size += p.length()
	}
	return v
}

func (v *tailVirtual) ReadAt(p []byte, off int64) (int, error) {
	v.asked += int64(len(p))
	v.biggest = max(v.biggest, len(p))
	if len(p) > v.maxReq {
		v.refused = true
		return 0, tailErrRefused
	}
	if off < 0 || off >= v.size {
		return 0, io.EOF
	}
	n := min(int64(len(p)), v.size-off)
	v.fill(p[:n], off)
	if n < int64(len(p)) {
		return int(n), io.EOF
	}
	return len(p), nil
}

// fill writes the bytes of [off, off+len(dst)) into dst, piece by piece.
func (v *tailVirtual) fill(dst []byte, off int64) {
	var start int64
	for _, pc := range v.pieces {
		end := start + pc.length()
		if len(dst) == 0 {
			return
		}
		if off < end {
			k := min(int64(len(dst)), end-off)
			out := dst[:k] // holds the file bytes [off, off+k)
			from := off - start
			switch {
			case pc.lit != nil:
				copy(out, pc.lit[from:from+k])
			case pc.solid:
				for i := 0; i < len(out); {
					i += copy(out[i:], tailSolid)
				}
			case pc.oneLine:
				for i := 0; i < len(out); {
					i += copy(out[i:], tailSolid)
				}
				if last := pc.n - 1; last >= from && last < from+k {
					out[last-from] = '\n'
				}
			default:
				tailFillLines(out, (start+from)%100)
			}
			dst = dst[k:]
			off += k
		}
		start = end
	}
}

// tailBlock is 40 copies of the filler line: 4000 bytes, a multiple of the 100-byte period, so
// a copy that ends at the block's end leaves the next write on a line boundary.
var tailBlock = func() []byte {
	b := make([]byte, 4000)
	for i := range b {
		b[i] = tailPattern[i%100]
	}
	return b
}()

// tailFillLines writes filler lines into out. phase is the global offset of out[0] mod 100.
func tailFillLines(out []byte, phase int64) {
	for i := 0; i < len(out); {
		i += copy(out[i:], tailBlock[phase:])
		phase = 0
	}
}

// tailLinePieces builds the pieces of a file of size bytes after prefix: filler lines whose
// lengths are drawn from [minLen, maxLen] by a generator with the given seed, the last one cut
// to fit. Every twentieth line is a refused needle line instead, so the scan meets the needle
// often and never accepts it.
func tailLinePieces(seed, size, minLen, maxLen int64, prefix ...tailPiece) []tailPiece {
	rng := rand.New(rand.NewSource(seed))
	pieces := append([]tailPiece(nil), prefix...)
	var total int64
	for _, p := range prefix {
		total += p.length()
	}
	refused := []byte(tailRejected[0] + "\n")
	for i := 0; total < size; i++ {
		if i%20 == 19 && total+int64(len(refused)) <= size {
			pieces = append(pieces, tailPiece{lit: refused})
			total += int64(len(refused))
			continue
		}
		n := min(minLen+rng.Int63n(maxLen-minLen+1), size-total)
		pieces = append(pieces, tailPiece{n: n, oneLine: true})
		total += n
	}
	return pieces
}

// tailFlaky serves reads until its after-th read, then fails every read with err.
type tailFlaky struct {
	data   []byte
	after  int
	err    error
	calls  int
	failed bool
}

func (f *tailFlaky) ReadAt(p []byte, off int64) (int, error) {
	f.calls++
	if f.calls > f.after {
		f.failed = true
		return 0, f.err
	}
	return copy(p, f.data[off:]), nil
}

// tailShort returns half of every request, with io.EOF when withEOF. It stops answering after a
// while, so a scan that ignores short reads cannot hang the test.
type tailShort struct {
	data    []byte
	withEOF bool
	calls   int
}

func (s *tailShort) ReadAt(p []byte, off int64) (int, error) {
	s.calls++
	if s.calls > 1<<16 {
		return 0, io.EOF
	}
	n := copy(p[:len(p)/2], s.data[off:])
	if s.withEOF {
		return n, io.EOF
	}
	return n, nil
}

// tailAlloc is the number of bytes f allocates, as the runtime counts them.
func tailAlloc(f func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	runtime.GC()
	return after.TotalAlloc - before.TotalAlloc
}

// tailMemCheck fails when a read was larger than the window, or when the scan allocated 8 MiB
// or more. The read check comes first: a refused read is the reason the scan failed.
func tailMemCheck(t *testing.T, what string, v *tailVirtual, alloc uint64) {
	t.Helper()
	if v.refused {
		t.Fatalf("%s: a read asked for %d bytes; a scan may read at most the %d-byte window",
			what, v.biggest, v.maxReq)
	}
	if alloc >= 8<<20 {
		t.Fatalf("%s: the scan allocated %d bytes, want under 8 MiB", what, alloc)
	}
}

func TestTailBoundedIO(t *testing.T) {
	acc := tailAccepted(37.5)
	accLine := acc + "\n"
	want, ok := ParseCodexLine(acc, tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}

	// A 1 GiB file with the accepted line 600 KB from EOF. The scan reads three windows and
	// finds it; reading the whole file is what this must rule out.
	t.Run("1 GiB file, line 600 KB from EOF", func(t *testing.T) {
		const size = int64(1) << 30
		const after = int64(600_000)
		head := (size - after - int64(len(accLine))) / 100 * 100
		v := tailNewVirtual(codexTailWindow,
			tailPiece{n: head},
			tailPiece{lit: []byte(accLine)},
			tailPiece{n: size - head - int64(len(accLine))})
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "1 GiB file", v, alloc)
		tailCheck(t, "1 GiB file", got, gok, err, want, true)
		if v.asked > 4*codexTailWindow {
			t.Fatalf("1 GiB file: %d bytes requested, want at most %d", v.asked, 4*codexTailWindow)
		}
	})

	// A 64 MiB file with no needle at all. The scan has to read all of it; what must stay
	// bounded is the memory it takes.
	t.Run("64 MiB file, no limits", func(t *testing.T) {
		const size = int64(64) << 20
		v := tailNewVirtual(codexTailWindow, tailPiece{n: size})
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "64 MiB file", v, alloc)
		tailCheck(t, "64 MiB file", got, gok, err, Reading{}, false)
	})

	// The 64 MiB final line is valid JSON and must win over the preceding record.
	t.Run("64 MiB final line over the cap", func(t *testing.T) {
		v := tailNewVirtual(codexTailWindow,
			tailPiece{n: 1000},
			tailPiece{lit: []byte(accLine)},
			tailPiece{lit: []byte(tailPadOpen)},
			tailPiece{n: 64 << 20, solid: true},
			tailPiece{lit: []byte(tailPadClose)})
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, v.size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "64 MiB final line", v, alloc)
		largeWant, largeOK := ParseCodexLine(tailPadded(100<<10), tailNow)
		tailCheck(t, "64 MiB final line", got, gok, err, largeWant, largeOK)
	})

	// 64 MiB of filler lines of 1,000 to 200,000 bytes, seeded, so most lines are over the
	// 64 KiB cap and use the fallback, and the rest cross windows. A refused needle line comes
	// every twentieth line. Nothing is accepted: the scan must read it all within the bound.
	t.Run("64 MiB, lines of 1-200 KB, no needle", func(t *testing.T) {
		const size = int64(64) << 20
		v := tailNewVirtual(codexTailWindow, tailLinePieces(7, size, 1_000, 200_000)...)
		if v.size != size {
			t.Fatalf("fixture: %d bytes, want %d", v.size, size)
		}
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, v.size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "64 MiB of 1-200 KB lines", v, alloc)
		tailCheck(t, "64 MiB of 1-200 KB lines", got, gok, err, Reading{}, false)
	})

	// The same, with every line under the cap: 1,000 to 30,000 bytes. Each line is copied into
	// the carry buffer across windows, which is where a per-line copy would show.
	t.Run("64 MiB, lines of 1-30 KB, no needle", func(t *testing.T) {
		const size = int64(64) << 20
		v := tailNewVirtual(codexTailWindow, tailLinePieces(8, size, 1_000, 30_000)...)
		if v.size != size {
			t.Fatalf("fixture: %d bytes, want %d", v.size, size)
		}
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, v.size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "64 MiB of 1-30 KB lines", v, alloc)
		tailCheck(t, "64 MiB of 1-30 KB lines", got, gok, err, Reading{}, false)
	})

	// The same lengths with the accepted line first in the file: the scan has to cross all of
	// the file to reach it, and then accept it from offset 0.
	t.Run("64 MiB, lines of 1-30 KB, accepted line first", func(t *testing.T) {
		const size = int64(64) << 20
		v := tailNewVirtual(codexTailWindow,
			tailLinePieces(9, size, 1_000, 30_000, tailPiece{lit: []byte(accLine)})...)
		if v.size != size {
			t.Fatalf("fixture: %d bytes, want %d", v.size, size)
		}
		var got Reading
		var gok bool
		var err error
		alloc := tailAlloc(func() { got, gok, err = lastRateLimits(v, v.size, tailNow, codexTailWindow, codexLineCap) })
		tailMemCheck(t, "64 MiB, accepted line first", v, alloc)
		tailCheck(t, "64 MiB, accepted line first", got, gok, err, want, true)
	})
}

func TestTailReadError(t *testing.T) {
	boom := errors.New("tail test: the disk went away")
	data := []byte(strings.Repeat(tailFiller+"\n", 64)) // no limits: a scan has to read all of it
	n := int64(len(data))

	// A reader that fails on every read: any scan that reads must report that error.
	f := &tailFlaky{data: data, after: 0, err: boom}
	_, ok, err := lastRateLimits(f, n, tailNow, 64, codexLineCap)
	if !f.failed {
		t.Fatal("fixture: the reader never failed")
	}
	if ok || !errors.Is(err, boom) {
		t.Errorf("reader failed on every read: ok=%v err=%v, want the read error", ok, err)
	}

	// A reader that fails after a few good reads. A scan that never reaches the failing read
	// (one that reads the file in a single call) has nothing to report; one that does reach it
	// must not swallow the error.
	for after := 1; after <= 5; after++ {
		f := &tailFlaky{data: data, after: after, err: boom}
		_, ok, err := lastRateLimits(f, n, tailNow, 64, codexLineCap)
		if f.failed && (ok || !errors.Is(err, boom)) {
			t.Errorf("reader failed after %d good reads: ok=%v err=%v, want the read error", after, ok, err)
		}
	}

	// A short read is io.ErrUnexpectedEOF, with or without an io.EOF from the reader.
	for _, withEOF := range []bool{true, false} {
		s := &tailShort{data: data, withEOF: withEOF}
		_, ok, err := lastRateLimits(s, n, tailNow, 64, codexLineCap)
		if ok || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("short read (io.EOF %v): ok=%v err=%v, want io.ErrUnexpectedEOF", withEOF, ok, err)
		}
	}
}

// tailUTF16 encodes text as a UTF-16 file with a byte order mark.
func tailUTF16(text string, be bool) []byte {
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2, 2+2*len(units))
	if be {
		out[0], out[1] = 0xFE, 0xFF
	} else {
		out[0], out[1] = 0xFF, 0xFE
	}
	for _, u := range units {
		if be {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// tailWriteUTF16Stream writes a UTF-16LE file of chunks blocks of 1,000,000 bytes: a byte order
// mark, then filler lines, then tail as a final line. Only one block is held at a time.
func tailWriteUTF16Stream(tb testing.TB, path string, chunks int, tail string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	line := tailUTF16(strings.Repeat("x", 99)+"\n", false)[2:] // 100 characters: 200 bytes
	block := bytes.Repeat(line, 5000)                          // 1,000,000 bytes
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	must := func(_ int, err error) {
		if err != nil {
			tb.Fatal(err)
		}
	}
	must(w.Write(tailUTF16("", false)))
	for i := 0; i < chunks; i++ {
		must(w.Write(block))
	}
	must(w.Write(tailUTF16(tail+"\n", false)[2:]))
	if err := w.Flush(); err != nil {
		tb.Fatal(err)
	}
}

func TestRolloutLimitsUTF16(t *testing.T) {
	acc := tailAccepted(37.5)
	if _, ok := ParseCodexLine(acc, tailNow); !ok {
		t.Fatal("fixture line is not accepted")
	}
	text := tailFiller + "\r\n" + acc + "\r\n"
	want, _ := ParseCodexLine(acc, tailNow)
	for _, be := range []bool{false, true} {
		name := "UTF-16LE"
		if be {
			name = "UTF-16BE"
		}
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		tailWriteFile(t, path, string(tailUTF16(text, be)))
		got, ok, err := rolloutLimits(path, tailNow)
		tailCheck(t, name+" file holding an accepted line", got, ok, err, want, true)
	}

	// The same text as UTF-8 yields the same accepted line.
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	tailWriteFile(t, path, text)
	got, ok, err := rolloutLimits(path, tailNow)
	tailCheck(t, "UTF-8 control", got, ok, err, want, true)

	if testing.Short() {
		return
	}
	// A 40 MiB UTF-16 file is streamed to disk in blocks. Both a tail hit and
	// a full-file miss must allocate much less than its size.
	big := filepath.Join(t.TempDir(), "big.jsonl")
	tailWriteUTF16Stream(t, big, 42, acc) // 42 blocks of 1,000,000 bytes, about 40 MiB
	var bigGot Reading
	var bigOK bool
	var bigErr error
	alloc := tailAlloc(func() { bigGot, bigOK, bigErr = rolloutLimits(big, tailNow) })
	if alloc >= 8<<20 {
		t.Fatalf("40 MiB UTF-16 file: rolloutLimits allocated %d bytes, want under 8 MiB", alloc)
	}
	tailCheck(t, "40 MiB UTF-16 file", bigGot, bigOK, bigErr, want, true)
	noMatch := filepath.Join(t.TempDir(), "no-match.jsonl")
	tailWriteUTF16Stream(t, noMatch, 42, tailFiller)
	alloc = tailAlloc(func() { bigGot, bigOK, bigErr = rolloutLimits(noMatch, tailNow) })
	if alloc >= 8<<20 {
		t.Fatalf("40 MiB UTF-16 file without a match: allocated %d bytes, want under 8 MiB", alloc)
	}
	tailCheck(t, "40 MiB UTF-16 no match", bigGot, bigOK, bigErr, Reading{}, false)
}

func TestRolloutLimitsUTF8Matches(t *testing.T) {
	raw := tailBOM + tailAccepted(10) + "\r\n" + tailFiller + "\r" + tailNoted("\xff bad") + "\n" + tailAccepted(20) + "\r\n"
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	tailWriteFile(t, path, raw)
	want, ok := ParseCodexLine(tailAccepted(20), tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	got, gok, err := rolloutLimits(path, tailNow)
	tailCheck(t, "UTF-8 file with a BOM and mixed endings", got, gok, err, want, true)
	if oracle, oracleOK := oracleBytes([]byte(raw), tailNow); !oracleOK || !reflect.DeepEqual(oracle, want) {
		t.Fatal("fixture: the oracle does not agree with the expected line")
	}
}

func TestRolloutLimitsUTF16Boundaries(t *testing.T) {
	for _, be := range []bool{false, true} {
		for _, ending := range []string{"", "\n", "\r", "\r\n"} {
			// Cross the 256 KiB read boundary with a valid record, while retaining
			// the final unpaired byte that the legacy UTF-16 decoder ignores.
			if ending == "" {
				ending = "\n"
			}
			record := tailAccepted(42)
			suffixChars := codexTailWindow/2 - len(record)/2 - len(ending)
			text := record + ending + strings.Repeat("x\n", suffixChars/2)
			raw := append(tailUTF16(text, be), 0x7f)
			path := filepath.Join(t.TempDir(), "rollout.jsonl")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			want, wok := oracleLast(core.TextOf(raw), tailNow)
			got, ok, err := rolloutLimits(path, tailNow)
			tailCheck(t, "UTF-16 boundary", got, ok, err, want, wok)
		}
	}
}

func TestRolloutLimitsUTF16LossyAndCodeUnits(t *testing.T) {
	for _, be := range []bool{false, true} {
		text := strings.Replace(tailAccepted(31), "{", "{\"ignored\":\"\u010a\u010d\ufffd\",", 1)
		raw := tailUTF16(text, be)
		// Replace U+FFFD with an unpaired high surrogate, which TextOf decodes lossily.
		old, replacement := []byte{0xfd, 0xff}, []byte{0x00, 0xd8}
		if be {
			old, replacement = []byte{0xff, 0xfd}, []byte{0xd8, 0x00}
		}
		raw = bytes.Replace(raw, old, replacement, 1)
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		want, wok := oracleLast(core.TextOf(raw), tailNow)
		if !wok {
			t.Fatal("legacy oracle rejected fixture")
		}
		got, ok, err := rolloutLimits(path, tailNow)
		tailCheck(t, "UTF-16 lossy code units", got, ok, err, want, wok)
	}
}

func TestRolloutLimitsTinyFiles(t *testing.T) {
	// A file shorter than a line, a byte order mark included, is no error: it reads as no limits.
	for _, text := range []string{"", "\n", "F", "\xff", "\xff\xfe", "\xfe\xff"} {
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		tailWriteFile(t, path, text)
		got, ok, err := rolloutLimits(path, tailNow)
		if ok || err != nil {
			t.Errorf("file %q: %s, err %v; want no reading and no error", text, tailShow(got, ok), err)
		}
	}
}

func TestRolloutLimitsMissingFileIsError(t *testing.T) {
	_, ok, err := rolloutLimits(filepath.Join(t.TempDir(), "absent.jsonl"), tailNow)
	if ok || err == nil {
		t.Fatalf("missing file: ok=%v err=%v, want no reading and an error", ok, err)
	}
}

// tailWriteFile writes text to path, creating its folders.
func tailWriteFile(tb testing.TB, path string, text string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// tailRolloutPath is where the large-rollout tests put their file under a CODEX_HOME.
func tailRolloutPath(home string) string {
	return filepath.Join(home, "sessions", "2026", "10", "29", "rollout-large.jsonl")
}

// tailWriteRollout streams a rollout of total bytes to path: filler lines, the accepted line
// acc and its terminator, then filler, leaving tailAfter bytes after the accepted line. It is
// written in chunks, so the test never holds the file in memory.
func tailWriteRollout(tb testing.TB, path string, total, tailAfter int64, acc string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		tb.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	accLine := acc + "\n"
	head := (total - tailAfter - int64(len(accLine))) / 100 * 100
	tailWriteFiller(tb, w, head)
	if _, err := w.WriteString(accLine); err != nil {
		tb.Fatal(err)
	}
	tailWriteFiller(tb, w, total-head-int64(len(accLine)))
	if err := w.Flush(); err != nil {
		tb.Fatal(err)
	}
}

// tailWriteFiller writes n bytes of filler lines, starting at a line boundary.
func tailWriteFiller(tb testing.TB, w io.Writer, n int64) {
	chunk := bytes.Repeat(tailPattern[:], 10_000)
	for n > 0 {
		k := min(n, int64(len(chunk)))
		if _, err := w.Write(chunk[:k]); err != nil {
			tb.Fatal(err)
		}
		n -= k
	}
}

func TestCodexInLargeRolloutBoundedAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 64 MiB rollout")
	}
	home := t.TempDir()
	acc := tailAccepted(37.5)
	tailWriteRollout(t, tailRolloutPath(home), 64<<20, 600_000, acc)
	want, ok := ParseCodexLine(acc, tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	var got Reading
	alloc := tailAlloc(func() { got = CodexIn(home, tailNow) })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CodexIn on a 64 MiB rollout: %s, want %s", tailShow(got, got.Used != nil), tailShow(want, true))
	}
	if alloc >= 16<<20 {
		t.Fatalf("CodexIn on a 64 MiB rollout allocated %d bytes, want under 16 MiB", alloc)
	}
}

// TestCodexInSkipsEmptyRollout: the newest-ranked rollout holds no bytes, or one, and an older
// rollout holds an accepted line. The short file is not an error: the old loop read it as no
// limits and went on to the next file.
//
// Ranking goes through the real CodexIn: lastEvent finds no timestamp in these files, so each is
// dated by its modified time, and the short file, set an hour ahead, ranks first. The older file's
// last event is dated 1 October, so it ranks second. A CodexIn that fails on the short file never
// reaches the older one, and the test fails.
func TestCodexInSkipsEmptyRollout(t *testing.T) {
	for _, c := range []struct{ name, text string }{{"empty", ""}, {"one newline", "\n"}, {"one byte", "F"}} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			day := filepath.Join(home, "sessions", "2026", "10", "29")
			old := strings.Replace(tailAccepted(37.5), "2026-10-29T11:00:00Z", "2026-10-01T11:00:00Z", 1)
			tailWriteFile(t, filepath.Join(day, "rollout-old.jsonl"), old+"\n")
			short := filepath.Join(day, "rollout-new.jsonl")
			tailWriteFile(t, short, c.text)
			stamp := tailNow.Add(time.Hour)
			if err := os.Chtimes(short, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			got := CodexIn(home, tailNow)
			if got.Used == nil || *got.Used != 37.5 {
				t.Fatalf("CodexIn with a %s newest rollout: %s, want 37.5", c.name, tailShow(got, got.Used != nil))
			}
		})
	}
}

// TestCodexInAcceptedBeforeLongLine: the accepted line is followed by a line longer than the
// 64 KiB cap and then by short lines. Invalid long lines leave the older reading intact.
func TestCodexInAcceptedBeforeLongLine(t *testing.T) {
	acc := tailAccepted(37.5)
	want, ok := ParseCodexLine(acc, tailNow)
	if !ok {
		t.Fatal("fixture line is not accepted")
	}
	short := strings.Repeat(tailFiller+"\n", 10)
	for _, c := range []struct{ name, long string }{
		{"filler longer than the cap", strings.Repeat("q", 100<<10)},
		{"needle, then filler, longer than the cap", `"rate_limits"` + strings.Repeat("q", 100<<10)},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, "sessions", "2026", "10", "29", "rollout-a.jsonl")
			tailWriteFile(t, path, acc+"\n"+c.long+"\n"+short)
			got := CodexIn(home, tailNow)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("CodexIn: %s, want %s", tailShow(got, got.Used != nil), tailShow(want, true))
			}
		})
	}
}

func BenchmarkCodexInLarge(b *testing.B) {
	home := b.TempDir()
	tailWriteRollout(b, tailRolloutPath(home), 64<<20, 600_000, tailAccepted(37.5))
	if r := CodexIn(home, tailNow); r.Used == nil {
		b.Fatal("CodexIn found no reading in the benchmark rollout")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CodexIn(home, tailNow)
	}
}

// TestTailFixturesAreWhatTheyClaim keeps the fixtures honest: a test that calls a line
// accepted or refused must be right about it, or it proves nothing.
func TestTailFixturesAreWhatTheyClaim(t *testing.T) {
	accepted := []struct {
		name string
		line string
	}{
		{"tailAccepted", tailAccepted(37.5)},
		{"tailMinimal", tailMinimal(5)},
		{"tailPadded(67)", tailPadded(67)},
		{"tailPadded(70)", tailPadded(70)},
		{"tailPadded(71)", tailPadded(71)},
		{"tailNoted", tailNoted("ok")},
	}
	for _, a := range accepted {
		if _, ok := ParseCodexLine(a.line, tailNow); !ok {
			t.Errorf("%s is refused by ParseCodexLine", a.name)
		}
	}
	if n := len(tailMinimal(5)); n > 64 {
		t.Errorf("tailMinimal is %d bytes, want at most 64", n)
	}
	for _, c := range []struct {
		name string
		line string
		n    int
	}{{"tailPadded(67)", tailPadded(67), 67}, {"tailPadded(70)", tailPadded(70), 70}, {"tailPadded(71)", tailPadded(71), 71}} {
		if len(c.line) != c.n {
			t.Errorf("%s is %d bytes, want %d", c.name, len(c.line), c.n)
		}
	}
	for _, line := range tailRejected {
		if _, ok := ParseCodexLine(line, tailNow); ok {
			t.Errorf("tailRejected fixture %q is accepted by ParseCodexLine", line)
		}
	}
	esc := tailEscapedKey()
	if !strings.Contains(esc, "\\u0072") || strings.Contains(esc, `"rate_limits"`) {
		t.Errorf("tailEscapedKey %q must hold the escape and not the literal needle", esc)
	}
	if _, ok := ParseCodexLine(esc, tailNow); !ok {
		t.Errorf("tailEscapedKey is refused by ParseCodexLine")
	}
}
