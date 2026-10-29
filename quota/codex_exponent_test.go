package quota

import (
	"fmt"
	"strings"
	"testing"
)

// strconv stops adding exponent digits once the exponent reaches 10,000, so a number whose
// exponent has five digits is read with a smaller exponent than it states. The old reader
// went through strconv and so did the exponent; the streaming reader for lines over 64 KiB
// copies that cap. This pins the two together, because raising the cap in one place would
// change readings only for the huge lines nobody exercises by hand.
func TestOversizedExponentCapMatchesLegacy(t *testing.T) {
	pad := strings.Repeat("x", 70<<10) // pushes every line past the 64 KiB fast path
	line := func(num string) []byte {
		return []byte(`{"payload":{"rate_limits":{"primary":{"used_percent":` + num +
			`,"window_minutes":300}}},"pad":"` + pad + `"}` + "\n")
	}
	zeros := func(n int) string { return strings.Repeat("0", n) }

	// 0.<k zeros>5e<k+1> is exactly 5. It reads as 5 while the exponent is below the cap and
	// as 0 once it is not, and the two readers have to agree on every one of these.
	cases := map[string]string{}
	for _, k := range []int{9990, 9998, 9999, 10000, 10001, 20000, 100004} {
		cases[fmt.Sprintf("0.<%d zeros>5e%d", k, k+1)] = "0." + zeros(k) + fmt.Sprintf("5e%d", k+1)
	}
	for name, num := range map[string]string{
		"5e100005":  "5e100005",  // overflows to infinity: the value is absent
		"5e-100005": "5e-100005", // underflows to zero
		"5E+99999":  "5E+99999",
		"1e309":     "1e309",
		"1e-324":    "1e-324",
		"1e10000":   "1e10000",
	} {
		cases[name] = num
	}

	for name, num := range cases {
		raw := line(num)
		want, wok := oracleBytes(raw, tailNow)
		got, gok, err := tailRun(raw, codexTailWindow, codexLineCap)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if gok != wok || (gok && tailShow(got, gok) != tailShow(want, wok)) {
			t.Errorf("%s: streaming %s, legacy %s", name, tailShow(got, gok), tailShow(want, wok))
		}
	}

	// The case where the cap decides the answer: written value 5, read value 0.
	raw := line("0." + zeros(100004) + "5e100005")
	want, wok := oracleBytes(raw, tailNow)
	if !wok || want.Used == nil || *want.Used != 0 {
		t.Fatalf("legacy no longer reads the capped exponent as 0: %s", tailShow(want, wok))
	}
	got, gok, err := tailRun(raw, codexTailWindow, codexLineCap)
	if err != nil || !gok || got.Used == nil || *got.Used != 0 {
		t.Fatalf("streaming reads the capped exponent as %s, err %v; legacy gives 0", tailShow(got, gok), err)
	}
}
