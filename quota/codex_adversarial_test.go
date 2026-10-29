package quota

// Adversarial parity tests for the backward Codex scan. The oracle is the removed
// whole-file CodexIn loop, so any difference in the Reading, in ok or in the error is
// a regression against the legacy behaviour, which is the specification.

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"agent-quota/internal/core"
)

type advConfig struct{ window, lineCap int }

var (
	// A lineCap of 0 sends every non-empty line through the streaming projection.
	advSmall      = []advConfig{{codexTailWindow, codexLineCap}, {7, 0}, {64, 16}, {1, 1}, {4093, 0}}
	advBigConfigs = []advConfig{{codexTailWindow, codexLineCap}, {7, 0}, {4093, 0}, {65539, 1000}}
)

const advTS = "2026-10-29T10:00:00Z"

// advLegacy is the removed CodexIn loop applied to one file's bytes.
func advLegacy(raw []byte, now time.Time) (Reading, bool) {
	var last *Reading
	for _, l := range strings.Split(core.TextOf(raw), "\n") {
		for _, line := range strings.Split(l, "\r") {
			if !strings.Contains(line, `"rate_limits"`) {
				continue
			}
			if q, ok := ParseCodexLine(line, now); ok {
				last = &q
			}
		}
	}
	if last == nil {
		return Reading{}, false
	}
	return *last, true
}

// advTail picks the decoder by BOM as rolloutLimits does, without a file.
func advTail(raw []byte, window, lineCap int) (Reading, bool, error) {
	r := bytes.NewReader(raw)
	if len(raw) >= 2 && (raw[0] == 0xff && raw[1] == 0xfe || raw[0] == 0xfe && raw[1] == 0xff) {
		return lastRateLimitsUTF16(r, int64(len(raw)), tailNow, [2]byte{raw[0], raw[1]})
	}
	return lastRateLimits(r, int64(len(raw)), tailNow, window, lineCap)
}

func advShow(r Reading, ok bool) string {
	s := fmt.Sprintf("ok=%v %+v", ok, r)
	if r.Used != nil {
		s += fmt.Sprintf(" used=%v", *r.Used)
	}
	if len(s) > 500 {
		s = s[:500] + "..."
	}
	return s
}

func advSame(got Reading, ok bool, err error, want Reading, wantOK bool) bool {
	return err == nil && ok == wantOK && reflect.DeepEqual(got, want)
}

func advCompare(t *testing.T, name string, raw []byte, configs []advConfig) {
	t.Helper()
	want, wantOK := advLegacy(raw, tailNow)
	for _, c := range configs {
		got, ok, err := advTail(raw, c.window, c.lineCap)
		if !advSame(got, ok, err, want, wantOK) {
			t.Errorf("%s (window %d, cap %d):\n got    %s err=%v\n legacy %s", name, c.window, c.lineCap, advShow(got, ok), err, advShow(want, wantOK))
		}
	}
}

func advConfigsFor(raw string) []advConfig {
	if len(raw) > codexLineCap {
		return advBigConfigs
	}
	return advSmall
}

// advLine is a token_count event whose rate_limits value is limits verbatim.
func advLine(ts, limits string) string {
	return `{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","rate_limits":` + limits + `}}`
}

func advRecord(used string) string {
	return advLine(advTS, `{"primary":{"used_percent":`+used+`,"window_minutes":300,"resets_at":1793361600},"secondary":{"used_percent":7,"window_minutes":10080,"resets_in_seconds":7200}}`)
}

// advBig widens a line past codexLineCap with an ignored leading property full of
// multibyte text and escaped quotes.
func advBig(line string) string {
	if !strings.HasPrefix(line, "{") {
		return line + strings.Repeat(" ", codexLineCap+1)
	}
	return `{"zz":"` + strings.Repeat(`é\"x`, (codexLineCap+16)/5) + `",` + line[1:]
}

func advUnitsBytes(units []uint16, be bool) []byte {
	out := make([]byte, 0, 2*len(units))
	for _, u := range units {
		if be {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// advUTF16 is a BOM followed by strings (encoded) and []uint16 (raw code units).
func advUTF16(be bool, parts ...any) []byte {
	units := []uint16{0xfeff}
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			units = append(units, utf16.Encode([]rune(v))...)
		case []uint16:
			units = append(units, v...)
		}
	}
	return advUnitsBytes(units, be)
}

// advMisaligned is text whose UTF-16 bytes hold the encoded needle at an odd offset,
// so a byte search finds it although the decoded text does not contain it.
func advMisaligned(be bool) []uint16 {
	needle := advUnitsBytes(utf16.Encode([]rune(`"rate_limits"`)), be)
	b := append(append([]byte{0x41}, needle...), 0x41)
	units := make([]uint16, len(b)/2)
	for i := range units {
		if be {
			units[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
		} else {
			units[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
	}
	return units
}

func advLines() []string {
	var out []string
	add := func(s ...string) { out = append(out, s...) }
	lim := func(limits ...string) {
		for _, l := range limits {
			add(advLine(advTS, l))
		}
	}
	nums := []string{
		"50", "1e308", "-0", "-0.0", "0e400", "-0e-400", "1e400", "-1e400", "1e-400", "-1e-400",
		"4.9e-324", "2.4703282292062327e-324", "2.4703282292062328e-324", "1.7976931348623157e308",
		"1.7976931348623159e308", "123456789012345678901234567890", "0.1e1", "1E2", "1e+2",
		"99.99999999999999999999999999999", "100.0000000000000000000000001",
		"1e0000000000000000000000000000002", "1e99999999999999999999", "1e-99999999999999999999",
		strings.Repeat("9", 900), "1" + strings.Repeat("0", 899) + "e-500",
		"1" + strings.Repeat("0", 898) + "1e-500", "5" + strings.Repeat("0", 999) + "e-700",
		"0." + strings.Repeat("0", 400) + "1e450", strings.Repeat("1", 30) + "e-29",
		"9007199254740993", "0.30000000000000004441", "1793361600.5", "253402300799", "253402300800",
		"-62135596801", "3.2e11", "320000000000.0005", "-3.2e11",
		`"50"`, "01", "1.", ".5", "+1", "-", "1e", "1e+", "NaN", "Infinity", "1_000", "0x10", "-01",
		"true", "null", "{}", "[]", `{"used_percent":5}`,
	}
	for _, n := range nums {
		lim(`{"primary":{"used_percent":`+n+`}}`,
			`{"primary":{"used_percent":40,"resets_at":`+n+`,"resets_in_seconds":60}}`,
			`{"primary":{"used_percent":40,"resets_in_seconds":`+n+`}}`,
			`{"primary":{"used_percent":40,"window_minutes":`+n+`}}`)
	}
	// Duplicate keys: the last one wins and duplicate objects do not merge.
	lim(`{"primary":{"used_percent":10}},"rate_limits":{"secondary":{"used_percent":20}}`,
		`{"primary":{"used_percent":10,"used_percent":"x"}}`,
		`{"primary":{"used_percent":"x","used_percent":10}}`,
		`{"primary":{"used_percent":10},"primary":5}`,
		`{"primary":{"used_percent":10},"primary":{"window_minutes":300}}`,
		`{"primary":{"used_percent":10,"resets_at":1793361600,"resets_at":"x","resets_in_seconds":90}}`,
		`{"primary":{"used_percent":10,"resets_at":1e400,"resets_in_seconds":60}}`,
		`{"primary":{"used_percent":10},"secondary":{"used_percent":30},"secondary":null}`)
	add(`{"timestamp":"`+advTS+`","timestamp":"bad","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"timestamp":"bad","timestamp":"`+advTS+`","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5}}},"payload":7}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5}}},"payload":{"x":"rate_limits"}}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5}},"rate_limits":null}}`)
	// Escaped keys, case, and the needle where it is not a key.
	add(`{"x":"rate_limits","payload":{"rate_\u006cimits":{"primary":{"used_percent":33}}}}`,
		`{"payload":{"rate_\u006cimits":{"primary":{"used_percent":33}}}}`,
		`{"\u0070ayload":{"rate_limits":{"\u0070rimary":{"used_\u0070ercent":44}}},"\u0074imestamp":"`+advTS+`"}`,
		`{"payload":{"rate_limits":{"Primary":{"used_percent":5}}}}`,
		`{"payload":{"rate_limits ":{"primary":{"used_percent":5}},"x":"rate_limits"}}`,
		`{"payload":{"\"rate_limits\"":{"primary":{"used_percent":5}}}}`,
		`{"note":"\\","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"note":"\\\\","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"note":"rate_limits","payload":{"x":"\"rate_limits\""}}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5,"note":"\\\"rate_limits\\"}}}}`,
		`{"payload":{"x":{"rate_limits":{"primary":{"used_percent":5}}}}}`)
	for _, ts := range []string{
		advTS, " " + advTS + " ", `\u00a0` + advTS + `\u2028`, "\u00a0" + advTS + "\u3000", `\u0000`,
		`\u0020` + advTS, "2026-10-29T10:00:00.0000000000000000000001Z", "2026-10-29T10:00:00.000000000000000000001Z",
		"2026-10-29T10:00:00.1234567Z", "2026-10-29 10:00:00Z", "2026-10-29T10:00:00+05:30",
		`\u0032026-10-29T10:00:00Z`, "2026-10-29T10:00:00Z\xff", "\xff",
		strings.Repeat(" ", 300) + advTS + strings.Repeat(`\t`, 300), advTS + `\"`, `\/` + advTS,
		"2026-10-29T10:00:00\u00a0Z", "\t" + advTS, advTS + `\u0000`,
	} {
		add(advLine(ts, `{"primary":{"used_percent":20,"resets_in_seconds":600}}`))
	}
	for _, v := range []string{"1793275200", `{"a":"` + advTS + `"}`, `["` + advTS + `"]`, "null", "true"} {
		add(`{"timestamp":` + v + `,"payload":{"rate_limits":{"primary":{"used_percent":5,"resets_in_seconds":60}}}}`)
	}
	for k := 59; k <= 65; k++ {
		arr := strings.Repeat("[", k) + strings.Repeat("]", k)
		obj := strings.Repeat(`{"a":`, k) + "1" + strings.Repeat("}", k)
		add(`{"deep":`+arr+`,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"deep":`+obj+`,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"payload":{"rate_limits":{"primary":{"used_percent":5,"deep":`+arr+`}}}}`)
	}
	base := advRecord("33")
	for _, w := range []string{" ", "\t", " \t ", "\v", "\f", "\x00", "\u00a0", "\u2028", "\u3000"} {
		add(w+base, base+w)
	}
	spaced := `{ "timestamp" : "` + advTS + `" , "payload" : { "rate_limits" : { "primary" : { "used_percent" : 12 , "resets_in_seconds" : 60 } } } }`
	add(spaced, strings.ReplaceAll(spaced, " ", "\t"), strings.ReplaceAll(spaced, " ", " \t  "))
	for _, g := range []string{"x", "{}", "}", ",", " 1", `"`, "\xff", "\xef\xbb\xbf", "\u00a0", "/"} {
		add(base + g)
	}
	add(base[:len(base)-1],
		`{"a":1,"payload":{"rate_limits":{"primary":{"used_percent":5}}},}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5,}}}}`,
		`{"payload" {"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"payload":{"rate_limits":{"primary":{"used_percent":5}}}}}`,
		`{payload:{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":tru,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":nulll,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":"unterminated,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":"a`+"\x01"+`b","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":/*c*/1,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":[1,],"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":[,1],"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		`{"x":1 2,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`)
	for _, esc := range []string{`\/`, `\b`, `\u0022`, `\\\"`, `\ud800`, `\udc00`, `\ud800\u0041`, `\ud800\udbff`,
		`\ud83d\ude00`, `\ud83d`, `\ud83d\`, `\x`, `\u12G4`, `\u12`, `\U0041`, `\'`, `\a`, "\\\x00", `\uDBFF\uDFFF`} {
		add(`{"note":"`+esc+`","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"`+esc+`":1,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			advLine(advTS+esc, `{"primary":{"used_percent":5}}`))
	}
	for _, bad := range []string{"\xff", "\xe2\x82", "\xc0\xaf", "\xed\xa0\x80", "\xf4\x90\x80\x80", "\x80", "\xef\xbf\xbd"} {
		add(`{"note":"`+bad+`","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"`+bad+`":1,"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`+bad,
			bad+`{"payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
			`{"payload":{"rate_limits":{"primary":{"used_percent":5`+bad+`}}}}`)
	}
	add(`["rate_limits"]`, `"rate_limits"`, `{"rate_limits":{"primary":{"used_percent":5}}}`,
		`{"payload":{"rate_limits":[{"primary":{"used_percent":5}}]}}`, `{"payload":{"rate_limits":null}}`,
		`{"payload":{"rate_limits":"x"}}`, `{"payload":"rate_limits"}`,
		`{"payload":{"rate_limits":{"primary":[{"used_percent":5}]}}}`,
		`{"payload":{"rate_limits":{"primary":null,"secondary":{"used_percent":9}}}}`,
		`{"payload":{"rate_limits":{}}}`, `{"payload":{"rate_limits":{"primary":{}}}}`)
	return out
}

func TestCodexAdversarialTargeted(t *testing.T) {
	older := advRecord("11")
	// Check the oracle itself: a valid newer record over 64 KiB beats an older one.
	oldOnly, _ := advLegacy([]byte(older), tailNow)
	if r, ok := advLegacy([]byte(older+"\n"+advBig(advRecord("66"))), tailNow); !ok || reflect.DeepEqual(r, oldOnly) {
		t.Fatalf("legacy oracle did not take the newer oversized record: %s", advShow(r, ok))
	}
	for i, line := range advLines() {
		for _, v := range []string{line, advBig(line)} {
			for j, raw := range []string{older + "\n" + v, older + "\r\n" + v + "\r\n", v} {
				advCompare(t, fmt.Sprintf("line %d form %d size %d %.160q", i, j, len(v), line), []byte(raw), advConfigsFor(raw))
			}
		}
	}
}

func TestCodexAdversarialBOM(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	a, b := advRecord("21"), advRecord("22")
	for i, raw := range []string{
		bom + a, bom + bom + a, a + "\n" + bom + b, bom, bom + "\n" + a, bom + " " + a, "\xef\xbb" + a,
		bom + "\xef\xbb" + a, bom + advBig(a), bom + bom + advBig(a), bom + advBig(a) + "\rx",
		advBig(a) + "\n" + bom + b, bom + a + "\n" + bom + b, bom + "\r\n" + advBig(a), "\xef" + a, bom + "\xff" + a,
		bom + advBig(a) + "\n" + bom + advBig(b),
	} {
		advCompare(t, fmt.Sprintf("bom case %d %.80q", i, raw), []byte(raw), advConfigsFor(raw))
	}
}

func TestCodexAdversarialUTF16Files(t *testing.T) {
	dir := t.TempDir()
	older := advRecord("11")
	big := func(used string) string { return advBig(advRecord(used)) }
	// straddle puts record across the boundary of the first backward window.
	straddle := func(record string) string {
		r := 2 * len(utf16.Encode([]rune(record)))
		return record + "\n" + strings.Repeat("y", (codexTailWindow-r/2)/2-1)
	}
	for _, be := range []bool{false, true} {
		mis := advMisaligned(be)
		cases := [][]any{
			{older, "\n", big("66")},
			{older, "\r\n", big("66"), "\r\n"},
			{older, "\n", `{"s":"`, []uint16{0xd800}, "x", []uint16{0xdc00}, []uint16{0xd800, 0x41}, []uint16{0xd83d, 0xde00}, `",` + big("67")[1:]},
			{older, "\n", `{"s":"`, []uint16{0xdbff}, []uint16{0xdfff, 0xdfff}, `",` + advRecord("68")[1:]},
			{older, "\n", `{"`, []uint16{0xd800}, `":1,` + advRecord("69")[1:]},
			{older, "\n", advRecord("70"), []uint16{0xd800}, "\n"},
			{older, "\n", advRecord("71"), []uint16{0xdbff}},
			{older, "\n", big("72"), []uint16{0xd800}},
			{older, "\n", straddle(advRecord("73"))},
			{older, "\n", straddle(big("74"))},
			{older, "\n", `{"s":"`, []uint16{0x0a0d, 0x0d0a, 0x0a00, 0x0d00, 0x2028}, `",` + advRecord("75")[1:]},
			{older, "\n", `{"s":"`, []uint16{0x0a0d, 0x0d0a, 0x0a00, 0x0d00}, `",` + big("76")[1:]},
			{older, "\n", `{"payload":{"x":"`, mis, `"}}`},
			{older, "\n", `{"zz":"` + strings.Repeat("x", 40000) + `","payload":{"x":"`, mis, `"}}`},
			{older, "\n", `{"m":"`, mis, `",` + advRecord("77")[1:]},
			{older, "\n", `{"m":"`, mis, `",` + big("78")[1:]},
			{older, "\r", advRecord("79"), "\r", "tail"},
			{older, "\n", "\ufeff" + advRecord("80")},
			{"\ufeff" + advRecord("81")},
			{},
			{"\n"},
			{big("82")},
			{older, "\n", advRecord("1" + strings.Repeat("0", 899) + "e-500")},
		}
		for i, parts := range cases {
			for _, odd := range []bool{false, true} {
				raw := advUTF16(be, parts...)
				if odd {
					raw = append(raw, 0x0a)
				}
				path := filepath.Join(dir, fmt.Sprintf("rollout-%d-%v-%v.jsonl", i, be, odd))
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatal(err)
				}
				want, wantOK := advLegacy(raw, tailNow)
				got, ok, err := rolloutLimits(path, tailNow)
				if !advSame(got, ok, err, want, wantOK) {
					t.Errorf("utf16 be=%v case %d odd=%v:\n got    %s err=%v\n legacy %s", be, i, odd, advShow(got, ok), err, advShow(want, wantOK))
				}
			}
		}
	}
}

func advNumber(rng *rand.Rand) string {
	if rng.Intn(25) == 0 {
		bad := []string{"01", "1.", ".5", "+1", "-", "1e", "1e+", "NaN", `"50"`, "null", "[]", "{}", "true"}
		return bad[rng.Intn(len(bad))]
	}
	digits := func(n int, zeros bool) string {
		b := make([]byte, n)
		for i := range b {
			switch {
			case i == 0:
				b[i] = byte('1' + rng.Intn(9))
			case zeros:
				b[i] = '0'
			default:
				b[i] = byte('0' + rng.Intn(10))
			}
		}
		return string(b)
	}
	var b strings.Builder
	if rng.Intn(4) == 0 {
		b.WriteByte('-')
	}
	switch rng.Intn(6) {
	case 0:
		b.WriteByte('0')
	case 1:
		b.WriteString(strconv.Itoa(rng.Intn(150)))
	case 2:
		b.WriteString(digits(1+rng.Intn(30), false))
	case 3:
		b.WriteString(digits(780+rng.Intn(60), false))
	case 4:
		b.WriteString(digits(780+rng.Intn(300), true))
	default:
		b.WriteString(digits(1+rng.Intn(25), false) + strings.Repeat("0", rng.Intn(1200)))
	}
	if rng.Intn(2) == 0 {
		b.WriteByte('.')
		b.WriteString(strings.Repeat("0", rng.Intn(3)*rng.Intn(500)))
		b.WriteString(strconv.Itoa(rng.Intn(1000000)))
	}
	if rng.Intn(2) == 0 {
		b.WriteByte("eE"[rng.Intn(2)])
		b.WriteString([]string{"", "+", "-"}[rng.Intn(3)])
		switch rng.Intn(5) {
		case 0:
			b.WriteString(strconv.Itoa(rng.Intn(31)))
		case 1:
			b.WriteString(strconv.Itoa(280 + rng.Intn(60)))
		case 2:
			b.WriteString(strconv.Itoa(300 + rng.Intn(1000)))
		case 3:
			b.WriteString("000" + strconv.Itoa(rng.Intn(400)))
		default:
			b.WriteString("1" + strings.Repeat("0", 4+rng.Intn(20)))
		}
	}
	return b.String()
}

func advPad(rng *rand.Rand, n int) string {
	pieces := []string{"lorem ipsum ", "é", "😀", `\n`, `\"`, `\\`, `\u00e9`, `\ud83d\ude00`, `\/`, "rate_limits", `\"rate_limits\"`}
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(pieces[rng.Intn(len(pieces))])
	}
	return b.String()
}

func advRandomRecord(rng *rand.Rand, pad int) string {
	names := []string{"used_percent", "resets_at", "resets_in_seconds", "window_minutes"}
	window := func() string {
		var parts []string
		for _, k := range rng.Perm(len(names)) {
			if rng.Intn(4) == 0 {
				continue
			}
			v := advNumber(rng)
			switch {
			case names[k] == "resets_at" && rng.Intn(2) == 0:
				v = strconv.Itoa(1793275200 + rng.Intn(400000) - 100000)
			case names[k] == "window_minutes" && rng.Intn(2) == 0:
				v = []string{"300", "10080", "45.5", "59", "60", "9999.99"}[rng.Intn(6)]
			case names[k] == "used_percent" && rng.Intn(2) == 0:
				v = strconv.Itoa(rng.Intn(120))
			}
			parts = append(parts, `"`+names[k]+`":`+v)
			if rng.Intn(8) == 0 {
				parts = append(parts, `"`+names[k]+`":`+advNumber(rng))
			}
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	limits := `{"primary":` + window()
	if rng.Intn(3) > 0 {
		limits += `,"secondary":` + window()
	}
	limits += "}"
	ts := []string{advTS, "2026-10-29T11:59:00Z", " 2026-10-29T08:00:00+02:00 ", "bad", `\u00a0` + advTS,
		"2026-10-29T10:00:00.12345678901234567890Z"}[rng.Intn(6)]
	pre := ""
	if pad > 0 {
		pre = `"pad":"` + advPad(rng, pad) + `",`
	}
	return `{` + pre + `"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","rate_limits":` + limits + `}}`
}

func advMutate(rng *rand.Rand, s string) string {
	if s == "" {
		return s
	}
	pos := func() int {
		if len(s) > 600 && rng.Intn(5) > 0 {
			if rng.Intn(2) == 0 {
				return rng.Intn(300)
			}
			return len(s) - 1 - rng.Intn(500)
		}
		return rng.Intn(len(s))
	}
	tokens := []string{"{", "}", "[", "]", `"`, ",", ":", `\`, "0", "e", "-", ".", " ", "\xff", "\x00", "\t", `\u`,
		"1e400", `"rate_limits"`, "\xef\xbb\xbf", `\ud800`}
	tok := tokens[rng.Intn(len(tokens))]
	switch rng.Intn(4) {
	case 0:
		return s[:pos()]
	case 1:
		i := pos()
		return s[:i] + tok + s[i+1:]
	case 2:
		i := pos()
		return s[:i] + tok + s[i:]
	default:
		return s + tok
	}
}

func advRandomLine(rng *rand.Rand) string {
	switch rng.Intn(10) {
	case 0:
		return ""
	case 1:
		return `{"timestamp":"` + advTS + `","type":"response_item","payload":{"text":"` + strings.Repeat("ok ", rng.Intn(50)) + `"}}`
	case 2, 3, 4:
		return advRandomRecord(rng, 0)
	case 5, 6:
		return advRandomRecord(rng, codexLineCap+rng.Intn(80<<10))
	default:
		return advMutate(rng, advRandomRecord(rng, rng.Intn(2)*(codexLineCap+rng.Intn(20<<10))))
	}
}

func advRandomFile(rng *rand.Rand) []byte {
	var b strings.Builder
	if rng.Intn(5) == 0 {
		b.WriteString("\xef\xbb\xbf")
	}
	for i, n := 0, 1+rng.Intn(6); i < n; i++ {
		if i > 0 {
			b.WriteString([]string{"\n", "\r", "\r\n", "\n\r", "\n\n"}[rng.Intn(5)])
		}
		b.WriteString(advRandomLine(rng))
	}
	if rng.Intn(2) == 0 {
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// TestCodexAdversarialRandomized is seeded differential fuzzing over UTF-8 and UTF-16
// files that mix small records, records over 64 KiB and mutated records.
func TestCodexAdversarialRandomized(t *testing.T) {
	files := 120
	if testing.Short() {
		files = 25
	}
	for seed := int64(1); seed <= int64(files); seed++ {
		rng := rand.New(rand.NewSource(seed))
		raw := advRandomFile(rng)
		advCompare(t, fmt.Sprintf("seed %d (%d bytes)", seed, len(raw)), raw, []advConfig{{codexTailWindow, codexLineCap}, {4093, 0}, {65539, 1000}})
		be := rng.Intn(2) == 0
		units := utf16.Encode([]rune(string(raw)))
		for k := rng.Intn(3); k > 0; k-- {
			i := rng.Intn(len(units) + 1)
			units = append(units[:i], append([]uint16{uint16(0xd800 + rng.Intn(0x800))}, units[i:]...)...)
		}
		u16 := advUTF16(be, units)
		if rng.Intn(3) == 0 {
			u16 = append(u16, byte(rng.Intn(256)))
		}
		advCompare(t, fmt.Sprintf("seed %d utf16 be=%v", seed, be), u16, []advConfig{{codexTailWindow, codexLineCap}})
	}
}

// FuzzCodexTailParity compares the tail scan with the legacy whole-file read. Under a
// plain `go test` it runs only the seed corpus.
func FuzzCodexTailParity(f *testing.F) {
	for _, s := range []string{
		advRecord("50"),
		advRecord("50") + "\r\n" + advRecord("1e400"),
		"\xef\xbb\xbf" + advRecord("12"),
		advRecord("1" + strings.Repeat("0", 899) + "e-500"),
		`{"payload":{"rate_limits":{"primary":{"used_percent":5}}},"payload":{"x":"rate_limits"}}`,
		advRecord("33") + "\n" + strings.Repeat("[", 70) + `"rate_limits"`,
		advRecord("34") + "\n" + `{"note":"\ud800","payload":{"rate_limits":{"primary":{"used_percent":5}}}}`,
		string(advUTF16(false, advRecord("44"), "\n", []uint16{0xd800}, advRecord("45"))),
		string(advUTF16(true, advRecord("46"), "\r", advRecord("47"))) + "\x00",
		string(advUTF16(false, `{"m":"`, advMisaligned(false), `"}`)),
	} {
		f.Add([]byte(s), uint16(7), uint16(0))
		f.Add([]byte(s), uint16(64), uint16(100))
	}
	f.Fuzz(func(t *testing.T, raw []byte, window, lineCap uint16) {
		want, wantOK := advLegacy(raw, tailNow)
		for _, c := range []advConfig{{int(window%512) + 1, int(lineCap % 512)}, {codexTailWindow, codexLineCap}} {
			got, ok, err := advTail(raw, c.window, c.lineCap)
			if !advSame(got, ok, err, want, wantOK) {
				t.Fatalf("window %d cap %d input %.300q:\n got    %s err=%v\n legacy %s", c.window, c.lineCap, raw, advShow(got, ok), err, advShow(want, wantOK))
			}
		}
	})
}
