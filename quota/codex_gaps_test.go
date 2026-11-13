package quota

// Gap tests for the oversized-line path of the Codex rollout scan (codex_stream.go) and
// the retry loop around it (codex_tail.go). The oracle is the legacy whole-file loop,
// advLegacy, as in codex_adversarial_test.go. Each scan is also checked against an
// expectation written here, so a wrong oracle cannot pass a wrong implementation.
//
// Inputs come from constants, so the file is deterministic. No t.Parallel: tailAlloc
// reads process-wide counters.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"agent-quota/internal/core"
)

// gapRec is an accepted record. It has no timestamp, so it is observed at tailNow.
const gapRec = `"payload":{"rate_limits":{"primary":{"used_percent":37}}}`

// gapNeedle is the quoted key that the raw prefilter searches for.
const gapNeedle = `"rate_limits"`

// JSON escapes are written as \x5c (a backslash) followed by the text, so the source
// holds no literal backslash-u sequence for a tool to decode.
const (
	gapEscR          = "\x5cu0072"          // r
	gapEscA          = "\x5cu0061"          // a
	gapEscUnderscore = "\x5cu005f"          // _
	gapEscQuote      = "\x5cu0022"          // "
	gapEscLoneHigh   = "\x5cud83d"          // a high surrogate with no low partner
	gapEscVT         = "\x5cu000b"          // vertical tab
	gapEscA41        = "\x5cu0041"          // A
	gapEscE9         = "\x5cu00e9"          // e acute
	gapEscPair       = "\x5cud83d\x5cude00" // a surrogate pair
	gapEscSpace      = "\x5cu0020"          // space
	gapEscNBSP       = "\x5cu00a0"          // no-break space
	gapEscLineSep    = "\x5cu2028"          // line separator
)

// gapConfigs is the production scan. The 4093-byte-window shapes are covered by the advSmall
// configurations in the existing suite, and the oversized path under test does not depend on
// the window.
var gapConfigs = []advConfig{{codexTailWindow, codexLineCap}}

// gapEnc names the encodings the way the scan numbers them: 0 UTF-8, 1 UTF-16LE, 2 UTF-16BE.
var gapEnc = []string{"utf-8", "utf-16le", "utf-16be"}

// gapWant is the outcome expected for a document: whether a reading exists, and its Used.
type gapWant struct {
	ok   bool
	used float64
}

// gapCase is one JSON object. gapRun checks it as an oversized line.
type gapCase struct {
	name string
	line string
	want gapWant // the outcome when the line stands alone
}

func gapX(n int) string { return strings.Repeat("x", n) }

func gapRange(lo, hi int) []int {
	var out []int
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

// gapNeedleAt is an accepted line of total characters whose only "rate_limits" starts at
// character index at. All characters are ASCII, so in UTF-16 the key starts at byte 2*at.
func gapNeedleAt(at, total, used int) string {
	const head, mid, tail = `{"pad":"`, `","payload":{`, `},"after":"`
	rec := fmt.Sprintf(`"rate_limits":{"primary":{"used_percent":%d}}`, used)
	pad := at - len(head) - len(mid)
	fill := total - at - len(rec) - len(tail) - len(`"}`)
	if pad < 0 || fill < 0 {
		panic(fmt.Sprintf("gapNeedleAt(%d, %d): does not fit", at, total))
	}
	return head + gapX(pad) + mid + rec + tail + gapX(fill) + `"}`
}

// gapOversize widens a JSON object past codexLineCap with an ignored leading string.
func gapOversize(line string) string {
	return `{"pad":"` + gapX(70<<10) + `",` + line[1:]
}

// gapEncode writes text in the encoding enc, with a byte order mark for UTF-16.
func gapEncode(text string, enc int) []byte {
	if enc == 0 {
		return []byte(text)
	}
	return tailUTF16(text, enc == 2)
}

// gapNeedleBytes is the quoted key in the encoding of the file.
func gapNeedleBytes(enc int) []byte {
	if enc == 0 {
		return []byte(gapNeedle)
	}
	return advUnitsBytes(utf16.Encode([]rune(gapNeedle)), enc == 2)
}

// gapCheckNeedleAt asserts that the last key in raw starts at byte want. A boundary test
// only means something when the key really sits where the test puts it.
func gapCheckNeedleAt(t *testing.T, what string, raw []byte, enc, want int) {
	t.Helper()
	if got := bytes.LastIndex(raw, gapNeedleBytes(enc)); got != want {
		t.Fatalf("%s: fixture key at byte %d, want %d", what, got, want)
	}
}

// gapHasOddKey reports whether the key bytes occur at an odd byte offset of raw. There a
// byte search finds them, although the decoded text does not contain the key.
func gapHasOddKey(raw, key []byte) bool {
	for i := 0; ; {
		j := bytes.Index(raw[i:], key)
		if j < 0 {
			return false
		}
		if (i+j)%2 == 1 {
			return true
		}
		i += j + 1
	}
}

// gapCheck scans raw at each config and requires the scan, the legacy oracle and want to
// agree. The oracle must already agree with want, so a wrong expectation fails the fixture.
func gapCheck(t *testing.T, what string, raw []byte, want gapWant, configs []advConfig) {
	t.Helper()
	legacy, lok := advLegacy(raw, tailNow)
	if lok != want.ok || (lok && (legacy.Used == nil || *legacy.Used != want.used)) {
		t.Fatalf("%s: fixture expects ok=%v used=%v; the legacy oracle gives %s",
			what, want.ok, want.used, advShow(legacy, lok))
	}
	for _, c := range configs {
		got, ok, err := advTail(raw, c.window, c.lineCap)
		if err != nil || ok != lok || (ok && !reflect.DeepEqual(got, legacy)) {
			t.Errorf("%s (window %d, cap %d):\n got    %s err=%v\n legacy %s",
				what, c.window, c.lineCap, advShow(got, ok), err, advShow(legacy, lok))
		}
	}
}

// gapRun checks each case as an oversized line in UTF-8 and both UTF-16 orders. Each line
// stands alone, ends with a terminator, or follows an accepted line. A rejected line falls
// back to that accepted line, whose Used is 11.
func gapRun(t *testing.T, cases []gapCase) {
	t.Helper()
	older := tailAccepted(11) + "\n"
	for _, c := range cases {
		body := gapOversize(c.line)
		after := c.want
		if !c.want.ok {
			after = gapWant{ok: true, used: 11}
		}
		shapes := []struct {
			name string
			text string
			want gapWant
		}{
			{"bare", body, c.want},
			{"terminated", body + "\n", c.want},
			{"after older", older + body + "\n", after},
		}
		for enc := range 3 {
			for _, s := range shapes {
				gapCheck(t, fmt.Sprintf("%s [%s, %s]", c.name, gapEnc[enc], s.name),
					gapEncode(s.text, enc), s.want, gapConfigs)
			}
		}
	}
}

// TestGapCodexStraddleUTF8 kills M7 (no overlap between scratch reads). The scan reads
// 32 KiB at a time, and each read after the first keeps len(key)-1 = 12 bytes of overlap,
// so the reads meet at 32768, 32768+32756 = 65524 and 98280. A mutant with no overlap
// meets at 32768, 65536 and 98304. A key that starts up to 12 bytes before a meeting
// point is missed by a mutant with no overlap.
func TestGapCodexStraddleUTF8(t *testing.T) {
	older := tailAccepted(11) + "\n"
	for _, b := range []int{32768, 65524, 65536, 98280, 98304} {
		total, ds := 200<<10, []int{0, 1, 6, 12}
		if b == 32768 {
			total, ds = 70<<10, gapRange(0, 12)
		}
		for _, d := range ds {
			for _, shape := range []struct{ name, prefix, suffix string }{
				{"bare", "", ""},
				{"terminated", "", "\n"},
				{"after older", older, "\n"},
			} {
				at := b - d
				raw := []byte(shape.prefix + gapNeedleAt(at, total, 37) + shape.suffix)
				what := fmt.Sprintf("key at %d (boundary %d, d %d, %s)", at, b, d, shape.name)
				gapCheckNeedleAt(t, what, raw, 0, len(shape.prefix)+at)
				gapCheck(t, what, raw, gapWant{true, 37}, gapConfigs)
			}
		}
	}
}

// TestGapCodexStraddleUTF16 is the UTF-16 form of TestGapCodexStraddleUTF8. A key is two
// bytes per character, so a key starts on an even byte offset from the line start. The
// overlap is 25 bytes, which puts the meeting points at 32768, 65511 and 98254 (bytes from
// the line start). The prefixes move the line start to 2 and 0 modulo 4 in the file, so both
// parities of the absolute offset are covered.
func TestGapCodexStraddleUTF16(t *testing.T) {
	older := tailAccepted(11) + "\n"
	for _, b := range []int{32768, 65511, 65536, 98254, 98304} {
		total, prefixes := 200<<10, []string{""}
		if b == 32768 {
			total, prefixes = 70<<10, []string{"", "\n", older}
		}
		for s := b - 26; s <= b; s++ {
			if s%2 != 0 {
				continue
			}
			for _, prefix := range prefixes {
				for enc := 1; enc <= 2; enc++ {
					raw := gapEncode(prefix+gapNeedleAt(s/2, total/2, 37), enc)
					what := fmt.Sprintf("%s key at byte %d (boundary %d, prefix %q)", gapEnc[enc], s, b, prefix)
					gapCheckNeedleAt(t, what, raw, enc, 2+2*len(prefix)+s)
					gapCheck(t, what, raw, gapWant{true, 37}, gapConfigs)
				}
			}
		}
	}
}

// TestGapCodexUTF16MisalignedKeys kills M10 (no code-unit alignment in the prefilter). The
// key's bytes occur at an odd offset, so a byte search finds them, but the decoded text
// has no key. With an escaped key as the only accepted record, the scan must not accept
// the line, because the legacy oracle sees no quoted key. The real record must still be
// accepted when misaligned keys surround it.
func TestGapCodexUTF16MisalignedKeys(t *testing.T) {
	older := tailAccepted(11) + "\n"
	pad := strings.Repeat("x", 40<<10)
	escaped := `"payload":{"` + gapEscR + `ate_limits":{"primary":{"used_percent":19}}}}`
	// The pad string's closing quote comes first, then the escaped record's own members.
	escRest := `",` + escaped
	for _, be := range []bool{false, true} {
		enc := 1
		if be {
			enc = 2
		}
		mis := advMisaligned(be)
		cases := []struct {
			name  string
			parts []any
			want  gapWant
		}{
			{"escaped key, misaligned before and after", []any{`{"pad":"`, mis, pad, mis, escRest}, gapWant{false, 0}},
			{"escaped key, misaligned after only", []any{`{"pad":"`, pad, mis, escRest}, gapWant{false, 0}},
			{"real key, misaligned before and after", []any{`{"pad":"`, mis, pad, mis, `",` + gapRec + `}`}, gapWant{true, 37}},
			{"real key, misaligned before only", []any{`{"pad":"`, mis, pad, `",` + gapRec + `}`}, gapWant{true, 37}},
			{"real key, misaligned after only", []any{`{"pad":"`, pad, mis, `",` + gapRec + `}`}, gapWant{true, 37}},
		}
		for _, c := range cases {
			// Behind an accepted older line, a rejected record falls back to it.
			afterOlder := gapWant{true, 11}
			if c.want.ok {
				afterOlder = c.want
			}
			for _, shape := range []struct {
				name  string
				parts []any
				want  gapWant
			}{
				{"bare", c.parts, c.want},
				{"after older", append([]any{older}, c.parts...), afterOlder},
			} {
				raw := advUTF16(be, shape.parts...)
				what := fmt.Sprintf("%s [%s, %s]", c.name, gapEnc[enc], shape.name)
				if !gapHasOddKey(raw, gapNeedleBytes(enc)) {
					t.Fatalf("%s: fixture has no misaligned key", what)
				}
				// A rejection must come from the key, not from broken JSON.
				if shape.name == "bare" {
					if _, err := core.ParseJSON(strings.TrimPrefix(core.TextOf(raw), "\xef\xbb\xbf")); err != nil {
						t.Fatalf("%s: fixture is not valid JSON: %v", what, err)
					}
				}
				gapCheck(t, what, raw, shape.want, gapConfigs)
			}
		}
	}
}

// TestGapCodexFalseLiteral kills M8 (the literal false broken in the stream parser). Only
// true and null were exercised before. The table covers false as an ignored value, in
// arrays and nested objects, as a window field, and as an absent used_percent, resets_at,
// window_minutes or timestamp. Truncated and near-miss spellings must be rejected. The
// "falsy" entry is the one a broken literal would wrongly accept.
func TestGapCodexFalseLiteral(t *testing.T) {
	cases := []gapCase{
		{"ignored false first", `{"flag":false,` + gapRec + `}`, gapWant{true, 37}},
		{"ignored false last", `{` + gapRec + `,"flag":false}`, gapWant{true, 37}},
		{"false in ignored array", `{"list":[false,true,null,false],` + gapRec + `}`, gapWant{true, 37}},
		{"false in nested object", `{"a":{"b":{"c":[{"d":false}]}},` + gapRec + `}`, gapWant{true, 37}},
		{"false beside primary", `{"payload":{"rate_limits":{"flag":false,"primary":{"used_percent":37}}}}`, gapWant{true, 37}},
		{"false inside primary", `{"payload":{"rate_limits":{"primary":{"used_percent":37,"flag":false}}}}`, gapWant{true, 37}},
		{"used_percent false is absent", `{"payload":{"rate_limits":{"primary":{"used_percent":false}}}}`, gapWant{false, 0}},
		{"used_percent false, secondary used", `{"payload":{"rate_limits":{"primary":{"used_percent":false},"secondary":{"used_percent":37}}}}`, gapWant{true, 37}},
		{"resets_at false falls back", `{"payload":{"rate_limits":{"primary":{"used_percent":37,"resets_at":false,"resets_in_seconds":60}}}}`, gapWant{true, 37}},
		{"window_minutes false", `{"payload":{"rate_limits":{"primary":{"used_percent":37,"window_minutes":false}}}}`, gapWant{true, 37}},
		{"timestamp false", `{"timestamp":false,` + gapRec + `}`, gapWant{true, 37}},
		{"payload false", `{"payload":false}`, gapWant{false, 0}},
		{"rate_limits false", `{"payload":{"rate_limits":false}}`, gapWant{false, 0}},
		{"truncated fals", `{"flag":fals,` + gapRec + `}`, gapWant{false, 0}},
		{"falsy", `{"flag":falsy,` + gapRec + `}`, gapWant{false, 0}},
		{"False", `{"flag":False,` + gapRec + `}`, gapWant{false, 0}},
		{"FALSE", `{"flag":FALSE,` + gapRec + `}`, gapWant{false, 0}},
		{"lone f", `{"flag":f,` + gapRec + `}`, gapWant{false, 0}},
		{"truncated fals in array", `{"list":[fals],` + gapRec + `}`, gapWant{false, 0}},
		{"falsefalse", `{"flag":falsefalse,` + gapRec + `}`, gapWant{false, 0}},
		{"truncated at end of line", `{` + gapRec + `,"flag":fals`, gapWant{false, 0}},
		{"false then unterminated", `{"flag":false,` + gapRec, gapWant{false, 0}},
	}
	gapRun(t, cases)
}

// TestGapCodexEscapesInOversizedStrings kills the \f mutant (M17) and checks \r, which no
// earlier oversized test used. Whitespace escapes in a timestamp are trimmed, as the legacy
// trim does. The timestamp must then be honoured, not replaced by the observation time.
func TestGapCodexEscapesInOversizedStrings(t *testing.T) {
	escapes := []string{`\"`, `\\`, `\/`, `\b`, `\f`, `\n`, `\r`, `\t`, gapEscVT, gapEscA41, gapEscE9, gapEscPair}
	var cases []gapCase
	for _, e := range escapes {
		cases = append(cases,
			gapCase{"ignored string with " + e, `{"note":"a` + e + `b",` + gapRec + `}`, gapWant{true, 37}},
			gapCase{"ignored key with " + e, `{"k` + e + `":1,` + gapRec + `}`, gapWant{true, 37}},
		)
	}
	cases = append(cases,
		gapCase{"lone high surrogate", `{"note":"` + gapEscLoneHigh + `",` + gapRec + `}`, gapWant{false, 0}},
		gapCase{"unknown escape", `{"note":"\q",` + gapRec + `}`, gapWant{false, 0}},
	)
	gapRun(t, cases)

	observed := time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)
	for _, e := range []string{`\f`, `\r`, `\t`, `\n`, gapEscVT, gapEscSpace, gapEscNBSP, gapEscLineSep} {
		line := `{"timestamp":"` + e + `2026-10-29T11:00:00Z` + e + `",` + gapRec + `}`
		raw := []byte(gapOversize(line))
		got, ok, err := advTail(raw, codexTailWindow, codexLineCap)
		if err != nil || !ok || !got.ObservedAt.Equal(observed) {
			t.Errorf("timestamp wrapped in %s: got %s err=%v observed=%v, want observed %v",
				e, advShow(got, ok), err, got.ObservedAt, observed)
		}
	}
}

// TestGapCodexEscapedKeyIsNotAKey covers an escaped spelling of the key. A JSON escape for
// the letter r (code point 0072) reads as rate_limits, but the quoted key is not among the file's bytes, and
// the legacy oracle reads the file text. So the line must be rejected, and a real record
// earlier or later must still win.
func TestGapCodexEscapedKeyIsNotAKey(t *testing.T) {
	const esc = `"payload":{"` + gapEscR + `ate_limits":{"primary":{"used_percent":19}}}`
	cases := []gapCase{
		{"escaped r", `{` + esc + `}`, gapWant{false, 0}},
		{"escaped a", `{"payload":{"r` + gapEscA + `te_limits":{"primary":{"used_percent":19}}}}`, gapWant{false, 0}},
		{"escaped underscore", `{"payload":{"rate` + gapEscUnderscore + `limits":{"primary":{"used_percent":19}}}}`, gapWant{false, 0}},
		{"escaped quotes in key", `{"payload":{"` + gapEscQuote + `rate_limits` + gapEscQuote + `":{"primary":{"used_percent":19}}}}`, gapWant{false, 0}},
	}
	// Each case must be valid JSON, so that its rejection can only come from the key.
	for _, c := range cases {
		if _, err := core.ParseJSON(c.line); err != nil {
			t.Fatalf("%s: fixture is not valid JSON: %v", c.name, err)
		}
	}
	gapRun(t, cases)
	good := gapOversize(`{` + gapRec + `}`)
	escaped := gapOversize(`{` + esc + `}`)
	for enc := range 3 {
		for _, text := range []string{good + "\n" + escaped + "\n", escaped + "\n" + good + "\n"} {
			gapCheck(t, fmt.Sprintf("real and escaped records, %s, %.30q", gapEnc[enc], text),
				gapEncode(text, enc), gapWant{true, 37}, gapConfigs)
		}
	}
}

// TestGapCodexTimestampLength kills M19 (the stream keeps a timestamp only up to 30 bytes,
// where ParseStamp reads up to 42). The 33-byte and 42-byte stamps are read, so the
// observation time comes from the stamp. The 43-byte stamp is refused, so the observation
// falls back to the scan's own time. Every case must also agree with the legacy reading.
func TestGapCodexTimestampLength(t *testing.T) {
	stamps := []string{
		"2026-10-29T10:00:00.1234567+05:30",
		"2026-10-29T10:00:00.1234567890123456+05:30",
		"2026-10-29T10:00:00.12345678901234567+05:30",
	}
	var cases []gapCase
	for _, s := range stamps {
		cases = append(cases, gapCase{fmt.Sprintf("%d-byte timestamp", len(s)), `{"timestamp":"` + s + `",` + gapRec + `}`, gapWant{true, 37}})
	}
	gapRun(t, cases)
	// 10:00 at +05:30 is 04:30 UTC. ParseStamp keeps seven fraction digits.
	read := time.Date(2026, 10, 29, 4, 30, 0, 123456700, time.UTC)
	for i, s := range stamps {
		got, ok, err := advTail(gapEncode(gapOversize(`{"timestamp":"`+s+`",`+gapRec+`}`), 0), codexTailWindow, codexLineCap)
		if err != nil || !ok {
			t.Fatalf("%d-byte timestamp: got %s err=%v", len(s), advShow(got, ok), err)
		}
		wantRead := i < 2
		if wantRead && !got.ObservedAt.Equal(read) || !wantRead && !got.ObservedAt.IsZero() {
			t.Errorf("%d-byte timestamp: observed %v, want %v (read=%v)", len(s), got.ObservedAt, read, wantRead)
		}
	}
}

// TestGapCodexOddStartUTF16 checks that the code-unit alignment in the oversized prefilter is
// measured from the line start, not from the file. The public path always starts a UTF-16
// line on an even file offset, so this calls oversizedCodex with one junk byte before the
// body, which puts the body on an odd offset. The reading must equal the legacy reading of
// the same body with its byte order mark.
func TestGapCodexOddStartUTF16(t *testing.T) {
	pad := strings.Repeat("x", 40<<10)
	escRec := `"payload":{"` + gapEscR + `ate_limits":{"primary":{"used_percent":19}}}}`
	for _, be := range []bool{false, true} {
		enc := 1
		if be {
			enc = 2
		}
		mis := advMisaligned(be)
		for _, c := range []struct {
			name  string
			parts []any
			ok    bool
		}{
			{"real key between misaligned keys", []any{`{"pad":"`, mis, pad, mis, `",` + gapRec + `}`}, true},
			{"escaped key between misaligned keys", []any{`{"pad":"`, mis, pad, mis, `",` + escRec}, false},
		} {
			file := advUTF16(be, c.parts...)
			want, wok := advLegacy(file, tailNow)
			if wok != c.ok {
				t.Fatalf("%s [%s]: fixture expects ok=%v, legacy gives %s", c.name, gapEnc[enc], c.ok, advShow(want, wok))
			}
			odd := append([]byte{'A'}, file[2:]...) // one junk byte, then the body without its BOM
			got, ok, err := oversizedCodex(bytes.NewReader(odd), 1, int64(len(odd)), enc, false, tailNow, make([]byte, 32<<10))
			if err != nil || ok != wok || (ok && !reflect.DeepEqual(got, want)) {
				t.Errorf("%s [%s]: odd start got %s err=%v, legacy %s", c.name, gapEnc[enc], advShow(got, ok), err, advShow(want, wok))
			}
		}
	}
}

// TestGapCodexOddStartStraddle is the odd-start form of the UTF-16 straddle sweep. The body
// starts at an odd file offset, and the keys straddle the first 32 KiB scratch read (the
// overlap is 25 bytes), so an overlap-free prefilter misses them. Both encodings are used.
func TestGapCodexOddStartStraddle(t *testing.T) {
	for _, be := range []bool{false, true} {
		enc := 1
		if be {
			enc = 2
		}
		for s := 32768 - 26; s <= 32768; s += 2 {
			file := advUTF16(be, gapNeedleAt(s/2, 35<<10, 37))
			gapCheckNeedleAt(t, fmt.Sprintf("%s key at byte %d, odd start", gapEnc[enc], s), file, enc, 2+s)
			want, wok := advLegacy(file, tailNow)
			if !wok {
				t.Fatalf("%s key at byte %d: legacy rejects the fixture", gapEnc[enc], s)
			}
			odd := append([]byte{'A'}, file[2:]...) // one junk byte, then the body without its BOM
			got, ok, err := oversizedCodex(bytes.NewReader(odd), 1, int64(len(odd)), enc, false, tailNow, make([]byte, 32<<10))
			if err != nil || ok != wok || !reflect.DeepEqual(got, want) {
				t.Errorf("%s key at byte %d, odd start: got %s err=%v, legacy %s", gapEnc[enc], s, advShow(got, ok), err, advShow(want, wok))
			}
		}
	}
}

// gapInfo is the FileInfo a fake rollout reports. Only Size is asked for.
type gapInfo struct {
	os.FileInfo
	size int64
}

func (i gapInfo) Size() int64 { return i.size }

// gapReadAt is bytes.Reader.ReadAt for a slice. A read that runs past the end is short
// with io.EOF, as the scan sees a file that was shrunk under it.
func gapReadAt(data, p []byte, off int64) (int, error) {
	if off >= int64(len(data)) {
		return 0, io.EOF
	}
	n := copy(p, data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// gapRollout reports the sizes in turn from Stat, and the last one thereafter.
type gapRollout struct {
	data  []byte
	sizes []int64
	stats int
}

func (f *gapRollout) Stat() (os.FileInfo, error) {
	i := min(f.stats, len(f.sizes)-1)
	f.stats++
	return gapInfo{size: f.sizes[i]}, nil
}

func (f *gapRollout) ReadAt(p []byte, off int64) (int, error) {
	return gapReadAt(f.data, p, off)
}

// TestGapRolloutScanPersistentShortRead kills M9 (a short read hidden after the rescans).
// Stat reports more bytes than ReadAt can supply on every try. rolloutScan must give up
// after codexScanTries scans and report io.ErrUnexpectedEOF, not return no reading with
// no error.
func TestGapRolloutScanPersistentShortRead(t *testing.T) {
	data := []byte(tailAccepted(9) + "\n")
	f := &gapRollout{data: data, sizes: []int64{int64(len(data)) + 100}}
	got, ok, err := rolloutScan(f, tailNow)
	if !errors.Is(err, io.ErrUnexpectedEOF) || ok || got.Used != nil {
		t.Fatalf("persistent short read: got %s err=%v, want io.ErrUnexpectedEOF and no reading", tailShow(got, ok), err)
	}
	if f.stats != codexScanTries {
		t.Fatalf("persistent short read: %d scans, want %d", f.stats, codexScanTries)
	}
}

// TestGapRolloutScanRetriesThenSucceeds checks the retry as a success path. The first
// codexScanTries-1 scans see a size larger than the data. The last scan sees the real size
// and must return the legacy reading with no error.
func TestGapRolloutScanRetriesThenSucceeds(t *testing.T) {
	data := []byte(tailAccepted(9) + "\n")
	sizes := make([]int64, codexScanTries)
	for i := range codexScanTries - 1 {
		sizes[i] = int64(len(data)) + 100
	}
	sizes[codexScanTries-1] = int64(len(data))
	f := &gapRollout{data: data, sizes: sizes}
	got, ok, err := rolloutScan(f, tailNow)
	want, wok := advLegacy(data, tailNow)
	if err != nil || ok != wok || !reflect.DeepEqual(got, want) {
		t.Fatalf("after short tries: got %s err=%v, want %s", tailShow(got, ok), err, tailShow(want, wok))
	}
	if f.stats != codexScanTries {
		t.Fatalf("after short tries: %d scans, want %d", f.stats, codexScanTries)
	}
}

// gapFailing fails its reads or its Stat with a fixed error, and is otherwise a file.
type gapFailing struct {
	data    []byte
	readErr error
	statErr error
	stats   int
}

func (f *gapFailing) Stat() (os.FileInfo, error) {
	f.stats++
	if f.statErr != nil {
		return nil, f.statErr
	}
	return gapInfo{size: int64(len(f.data))}, nil
}

func (f *gapFailing) ReadAt(p []byte, off int64) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return gapReadAt(f.data, p, off)
}

// TestGapRolloutScanOtherErrorsNotRetried checks that only shrink-shaped errors are
// rescanned. A failed read or Stat is reported on the first scan, since rescanning it
// would hide a real I/O fault.
func TestGapRolloutScanOtherErrorsNotRetried(t *testing.T) {
	data := []byte(tailAccepted(9) + "\n")
	read := errors.New("gap: injected read failure")
	f := &gapFailing{data: data, readErr: read}
	if _, ok, err := rolloutScan(f, tailNow); !errors.Is(err, read) || ok || f.stats != 1 {
		t.Fatalf("read failure: err=%v ok=%v scans=%d, want the read error after one scan", err, ok, f.stats)
	}
	stat := errors.New("gap: injected stat failure")
	g := &gapFailing{data: data, statErr: stat}
	if _, ok, err := rolloutScan(g, tailNow); !errors.Is(err, stat) || ok || g.stats != 1 {
		t.Fatalf("stat failure: err=%v ok=%v scans=%d, want the stat error after one scan", err, ok, g.stats)
	}
}

// gapEOFProbe answers a read that reaches the end of the data with io.EOF and the full
// count. io.ReaderAt allows this at the end of the input. os.File never does it: its
// ReadAt returns (n, nil) for an exact-end read.
type gapEOFProbe struct {
	data  []byte
	stats int
}

func (f *gapEOFProbe) Stat() (os.FileInfo, error) {
	f.stats++
	return gapInfo{size: int64(len(f.data))}, nil
}

func (f *gapEOFProbe) ReadAt(p []byte, off int64) (int, error) {
	if off >= 0 && off+int64(len(p)) == int64(len(f.data)) {
		return copy(p, f.data[off:]), io.EOF
	}
	return gapReadAt(f.data, p, off)
}

// TestGapRolloutScanBOMProbeEOFFinding records a finding and asserts nothing about io.EOF.
// Today, rolloutScanOnce returns the BOM probe's error as it is. io.EOF is one of the errors
// rolloutScan retries, so after three scans rolloutScan returns io.EOF. io.ReaderAt allows a
// reader to return (n, io.EOF) when a read ends exactly at the end of the input, and for a
// two-byte BOM-only file that read is the whole file. For such a reader the scan reports
// io.EOF where the legacy whole-file read reports no error. os.File does not do this (see
// TestGapRolloutLimitsBOMOnlyFile), so this is a robustness finding, not a Windows failure.
// The test checks only what both agree on: no reading.
func TestGapRolloutScanBOMProbeEOFFinding(t *testing.T) {
	for _, bom := range [][]byte{{0xff, 0xfe}, {0xfe, 0xff}} {
		f := &gapEOFProbe{data: bom}
		got, ok, err := rolloutScan(f, tailNow)
		t.Logf("finding (not asserted): BOM-only file %x, BOM probe (2, io.EOF): ok=%v err=%v scans=%d",
			bom, ok, err, f.stats)
		if ok || got.Used != nil {
			t.Fatalf("BOM-only file %x gave a reading %s", bom, tailShow(got, ok))
		}
	}
}

// TestGapRolloutLimitsBOMOnlyFile covers the same file on disk, where ReadAt is os.File's.
// Both encodings' BOM-only files must give no reading and no error, as the legacy read does.
func TestGapRolloutLimitsBOMOnlyFile(t *testing.T) {
	for _, bom := range [][]byte{{0xff, 0xfe}, {0xfe, 0xff}} {
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		if err := os.WriteFile(path, bom, 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok, err := rolloutLimits(path, tailNow)
		if err != nil || ok || got.Used != nil {
			t.Fatalf("BOM-only file %x: got %s err=%v, want no reading and no error", bom, tailShow(got, ok), err)
		}
	}
}

// gapShrink serves old until switchAt reads have been made, then cur. Stat reports the
// size of old once and then the size of cur, which is a rollout Codex rewrote shorter
// during one scan.
type gapShrink struct {
	old, cur []byte
	switchAt int
	stats    int
	reads    int
}

func (f *gapShrink) Stat() (os.FileInfo, error) {
	f.stats++
	if f.stats == 1 {
		return gapInfo{size: int64(len(f.old))}, nil
	}
	return gapInfo{size: int64(len(f.cur))}, nil
}

func (f *gapShrink) ReadAt(p []byte, off int64) (int, error) {
	f.reads++
	if f.reads > f.switchAt {
		return gapReadAt(f.cur, p, off)
	}
	return gapReadAt(f.old, p, off)
}

// TestGapCodexShrinkDuringOversizedScan covers the reads that come up short inside the
// oversized path. Each old file is one oversized line, and the switch happens before the
// read that fails. The UTF-8 case fails in the prefilter (read 3, after the BOM probe and
// the tail chunk). The UTF-16 cases fail in the tail chunk (read 2) and in the prefilter
// (read 3). The scan must report the short read to rolloutScan, which then rescans the
// shrunk file and returns its legacy reading.
func TestGapCodexShrinkDuringOversizedScan(t *testing.T) {
	line := gapOversize(`{` + gapRec + `}`)
	for _, c := range []struct {
		name     string
		enc      int
		switchAt int
	}{
		{"UTF-8 prefilter", 0, 2},
		{"UTF-16LE tail chunk", 1, 1},
		{"UTF-16LE prefilter", 1, 2},
		{"UTF-16BE tail chunk", 2, 1},
		{"UTF-16BE prefilter", 2, 2},
	} {
		f := &gapShrink{old: gapEncode(line, c.enc), cur: gapEncode(tailAccepted(11)+"\n", c.enc), switchAt: c.switchAt}
		got, ok, err := rolloutScan(f, tailNow)
		want, wok := advLegacy(f.cur, tailNow)
		if err != nil || ok != wok || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %s err=%v, want %s", c.name, tailShow(got, ok), err, tailShow(want, wok))
		}
		if f.stats != 2 {
			t.Errorf("%s: %d scans, want 2 (one short read, one rescan)", c.name, f.stats)
		}
	}
}
