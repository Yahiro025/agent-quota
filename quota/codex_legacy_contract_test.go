package quota

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"agent-quota/internal/core"
)

// These tests record the legacy parser's contract. They do not assert that the
// capped tail reader supports oversized records; that remains a separate defect.
func TestCodexLegacyOversizedContract(t *testing.T) {
	pad := strings.Repeat("x", 70<<10)
	base := `{"payload":{"rate_limits":{"primary":{"used_percent":7}}}}`
	cases := []struct {
		name string
		line string
		used float64
		ok   bool
	}{
		{"ignored long string", `{"pad":"` + pad + `",` + base[1:], 7, true},
		{"bad ignored escape", `{"pad":"` + pad + `\q",` + base[1:], 0, false},
		{"lone escaped surrogate", `{"pad":"` + pad + `\uD800",` + base[1:], 0, false},
		{"partial write", base[:len(base)-1] + `,"pad":"` + pad, 0, false},
		{"trailing data", base + ` "` + pad + `"`, 0, false},
		{"last duplicate field", `{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":7,"used_percent":13}}}}`, 13, true},
		{"last duplicate ancestor replaces", `{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":7}}},"payload":null}`, 0, false},
		{"underflow", `{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":1e-9999}}}}`, 0, true},
		{"overflow becomes absent", `{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":1e9999}}}}`, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := ParseCodexLine(tc.line, tailNow)
			if ok != tc.ok || (ok && (r.Used == nil || *r.Used != tc.used)) {
				t.Fatalf("got %s, want ok=%v used=%v", tailShow(r, ok), tc.ok, tc.used)
			}
		})
	}
}

func TestCodexLegacyNumericRoundingAndUTF16(t *testing.T) {
	// A decimal exactly halfway between 1 and its successor rounds to even;
	// a nonzero digit far beyond it changes the answer.
	const midpoint = "1.00000000000000011102230246251565404236316680908203125"
	for _, tc := range []struct {
		number string
		want   float64
	}{{midpoint, 1}, {midpoint + strings.Repeat("0", 70000) + "1", math.Nextafter(1, 2)}} {
		v, err := core.ParseJSON(tc.number)
		if err != nil {
			t.Fatal(err)
		}
		got, err := v.F64()
		if err != nil || got != tc.want {
			t.Fatalf("F64: got %.17g, %v; want %.17g", got, err, tc.want)
		}
	}
	line := `{"pad":"` + strings.Repeat("x", 70000) + `",` + tailAccepted(23)[1:]
	want, ok := ParseCodexLine(line, tailNow)
	if !ok {
		t.Fatal("legacy fixture rejected")
	}
	for _, be := range []bool{false, true} {
		got, ok := oracleLast(core.TextOf(tailUTF16(line, be)), tailNow)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("UTF-16 big endian=%v: got %s", be, tailShow(got, ok))
		}
	}
}
