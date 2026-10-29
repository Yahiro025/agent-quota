package quota

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"agent-quota/internal/core"
)

type codexFailReader struct {
	data  []byte
	calls int
	err   error
}

func (r *codexFailReader) ReadAt(b []byte, off int64) (int, error) {
	r.calls++
	if r.calls == 2 {
		n := copy(b, r.data[off:min(int64(len(r.data)), off+int64(len(b)/2))])
		return n, r.err
	}
	return bytes.NewReader(r.data).ReadAt(b, off)
}

func TestOversizedCodexSecondPassReadFailure(t *testing.T) {
	line := tailMinimal(23) + strings.Repeat(" ", 70<<10)
	for _, enc := range []int{0, 1, 2} {
		raw := []byte(line)
		if enc != 0 {
			raw = tailUTF16(line, enc == 2)[2:]
		}
		for _, failure := range []error{io.EOF, errors.New("injected read failure")} {
			r := &codexFailReader{data: raw, err: failure}
			_, ok, err := oversizedCodex(r, 0, int64(len(raw)), enc, false, tailNow, make([]byte, 32<<10))
			if ok || err == nil {
				t.Fatalf("encoding %d, failure %v: ok=%v err=%v", enc, failure, ok, err)
			}
		}
	}
}

func TestOversizedCodexFirstLineBOMProbeFailure(t *testing.T) {
	raw := []byte(tailMinimal(23) + strings.Repeat(" ", 70<<10))
	for _, failure := range []error{io.EOF, errors.New("injected read failure")} {
		r := &codexFailReader{data: raw, err: failure}
		_, ok, err := oversizedCodex(r, 0, int64(len(raw)), 0, true, tailNow, make([]byte, 32<<10))
		if ok || err == nil {
			t.Fatalf("failure %v: ok=%v err=%v", failure, ok, err)
		}
	}
}

func TestOversizedCodexIgnoredStringAllocation(t *testing.T) {
	line := `{"note":"` + strings.Repeat("x ", 4<<20) + `",` + tailMinimal(23)[1:]
	raw := []byte(line)
	var got Reading
	var ok bool
	var err error
	alloc := tailAlloc(func() {
		got, ok, err = lastRateLimits(bytes.NewReader(raw), int64(len(raw)), tailNow, codexTailWindow, codexLineCap)
	})
	if err != nil || !ok || got.Used == nil || *got.Used != 23 {
		t.Fatalf("got %s, %v", tailShow(got, ok), err)
	}
	if alloc > 2<<20 {
		t.Fatalf("ignored 8 MiB string allocated %d bytes", alloc)
	}
}

func TestOversizedCodexIgnoredPropertiesAllocation(t *testing.T) {
	line := "{" + strings.Repeat(`"ignored":0,`, 200000) + tailMinimal(23)[1:]
	raw := []byte(line)
	var got Reading
	var ok bool
	var err error
	alloc := tailAlloc(func() {
		got, ok, err = lastRateLimits(bytes.NewReader(raw), int64(len(raw)), tailNow, codexTailWindow, codexLineCap)
	})
	if err != nil || !ok || got.Used == nil || *got.Used != 23 {
		t.Fatalf("got %s, %v", tailShow(got, ok), err)
	}
	if alloc > 2<<20 {
		t.Fatalf("ignored properties allocated %d bytes", alloc)
	}
}

func TestOversizedCodexMatchesLegacy(t *testing.T) {
	pad := strings.Repeat("x", 70<<10)
	base := tailMinimal(25)
	cases := []string{
		`{"pad":"` + pad + `",` + base[1:],
		`{"pad":"` + pad + `\q",` + base[1:],
		`{"pad":"` + pad + `\uD800",` + base[1:],
		`{"pad":"` + pad + `",` + base[1:] + "\x00",
		`{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":1}}},"payload":null}`,
		`{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":1,"used_percent":42}}}}`,
		`{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"resets_in_seconds":3600},"secondary":{"used_percent":31}}}}`,
		`{"pad":"` + pad + `","timestamp":"` + strings.Repeat(" ", 1000) + `2026-10-29T11:00:00Z` + strings.Repeat(" ", 1000) + `",` + base[1:],
		`{"pad":"` + pad + `","note":"rate_limits","payload":{"\u0072ate_limits":{"primary":{"used_percent":19}}}}`,
		`{"pad":"` + pad + `",` + base[1:len(base)-1],
		`{"pad":"` + pad + `","nested":` + strings.Repeat("[", 64) + `0` + strings.Repeat("]", 64) + `,` + base[1:],
		`{"payload":{"rate_limits":{"primary":{"used_percent":1.00000000000000011102230246251565404236316680908203125` + strings.Repeat("0", 70000) + `1}}}}`,
		`{"pad":"` + pad + `","payload":{"rate_limits":{"primary":{"used_percent":1e99999999999},"secondary":{"used_percent":13}}}}`,
	}
	for i, line := range cases {
		for enc := 0; enc < 3; enc++ {
			raw := []byte(line)
			if enc > 0 {
				raw = tailUTF16(line, enc == 2)
			}
			want, wok := oracleLast(core.TextOf(raw), tailNow)
			var got Reading
			var ok bool
			var err error
			if enc == 0 {
				got, ok, err = lastRateLimits(bytes.NewReader(raw), int64(len(raw)), tailNow, 4096, codexLineCap)
			} else {
				got, ok, err = lastRateLimitsUTF16(bytes.NewReader(raw), int64(len(raw)), tailNow, [2]byte{raw[0], raw[1]})
			}
			if err != nil || ok != wok || (ok && !reflect.DeepEqual(got, want)) {
				t.Fatalf("case %d encoding %d: got %s err=%v; want %s", i, enc, tailShow(got, ok), err, tailShow(want, wok))
			}
		}
	}
}

func TestStreamNumberLegacyParity(t *testing.T) {
	rng := rand.New(rand.NewSource(7719))
	inputs := []string{"1.00000000000000011102230246251565404236316680908203125" + strings.Repeat("0", 70000) + "1", "1" + strings.Repeat("0", 2000) + "e-2000", "0." + strings.Repeat("0", 2000) + "1e2001", "1" + strings.Repeat("2", 1200) + "e-99999", "-0.0000e9999999"}
	for i := 0; i < 2500; i++ {
		n := 2 + rng.Intn(1299)
		var b strings.Builder
		if rng.Intn(2) == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(byte('1' + rng.Intn(9)))
		if rng.Intn(2) == 0 {
			b.WriteByte('.')
		}
		for j := 1; j < n; j++ {
			b.WriteByte(byte('0' + rng.Intn(10)))
		}
		b.WriteByte('e')
		b.WriteString(strconv.Itoa(rng.Intn(3001) - 1500))
		inputs = append(inputs, b.String())
	}
	for i := 0; i < 2500; i++ {
		before, after := 1+rng.Intn(1100), 1+rng.Intn(1100)
		var b strings.Builder
		if rng.Intn(2) == 0 {
			b.WriteByte('-')
		}
		if rng.Intn(2) == 0 {
			b.WriteByte('0')
		} else {
			b.WriteByte(byte('1' + rng.Intn(9)))
			for j := 1; j < before; j++ {
				b.WriteByte(byte('0' + rng.Intn(10)))
			}
		}
		b.WriteByte('.')
		for j := 0; j < after; j++ {
			b.WriteByte(byte('0' + rng.Intn(10)))
		}
		b.WriteByte('e')
		b.WriteString(strconv.Itoa(rng.Intn(4001) - 2000))
		inputs = append(inputs, b.String())
	}
	for i, s := range inputs {
		p := &codexStream{r: bufio.NewReader(strings.NewReader(s))}
		v := p.number(true)
		f, err := strconv.ParseFloat(s, 64)
		got, gerr := v.F64()
		if p.bad || ((err == nil) != (gerr == nil)) || (err == nil && math.Float64bits(f) != math.Float64bits(got)) {
			t.Fatalf("case %d length %d: got %.17g (%v), want %.17g (%v)", i, len(s), got, gerr, f, err)
		}
	}
}
