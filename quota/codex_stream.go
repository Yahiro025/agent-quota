package quota

// The oversized-record path projects the fields Codex uses while validating
// every value. Its storage is independent of the length of ignored values.
import (
	"bufio"
	"bytes"
	"io"
	"math"
	"strconv"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"agent-quota/internal/core"
)

// utf16Reader decodes the same unpaired code units as core.TextOf.
type utf16Reader struct {
	r          *bufio.Reader
	le         bool
	pending    uint16
	hasPending bool
	out        [utf8.UTFMax]byte
	n          int
}

type exactSection struct {
	r        io.ReaderAt
	pos, end int64
}

func (s *exactSection) Read(b []byte) (int, error) {
	if s.pos >= s.end {
		return 0, io.EOF
	}
	if int64(len(b)) > s.end-s.pos {
		b = b[:s.end-s.pos]
	}
	n, err := s.r.ReadAt(b, s.pos)
	s.pos += int64(n)
	if n < len(b) {
		if err == nil || err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return n, err
	}
	return n, nil
}

func (r *utf16Reader) unit() (uint16, error) {
	if r.hasPending {
		r.hasPending = false
		return r.pending, nil
	}
	a, err := r.r.ReadByte()
	if err != nil {
		return 0, err
	}
	b, err := r.r.ReadByte()
	if err != nil {
		if err == io.EOF {
			return 0, io.EOF
		}
		return 0, err
	}
	if r.le {
		return uint16(a) | uint16(b)<<8, nil
	}
	return uint16(a)<<8 | uint16(b), nil
}
func (r *utf16Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	for n < len(p) {
		if r.n > 0 {
			k := copy(p[n:], r.out[:r.n])
			copy(r.out[:], r.out[k:r.n])
			r.n -= k
			n += k
			continue
		}
		u, err := r.unit()
		if err != nil {
			return n, err
		}
		v := rune(u)
		if u >= 0xd800 && u < 0xdc00 {
			w, e := r.unit()
			if e == nil {
				if w >= 0xdc00 && w < 0xe000 {
					v = utf16.DecodeRune(rune(u), rune(w))
				} else {
					r.pending = w
					r.hasPending = true
					v = utf8.RuneError
				}
			} else if e == io.EOF {
				v = utf8.RuneError
			} else {
				return n, e
			}
		} else if u >= 0xdc00 && u < 0xe000 {
			v = utf8.RuneError
		}
		r.n = utf8.EncodeRune(r.out[:], v)
	}
	return n, nil
}

type codexStream struct {
	r   *bufio.Reader
	err error
	bad bool
}

func (p *codexStream) peek() byte {
	b, e := p.r.Peek(1)
	if e != nil {
		if e != io.EOF {
			p.err = e
		}
		return 0
	}
	return b[0]
}
func (p *codexStream) take() byte {
	b, e := p.r.ReadByte()
	if e != nil {
		if e != io.EOF {
			p.err = e
		}
		p.bad = true
	}
	return b
}
func (p *codexStream) ws() {
	for {
		switch p.peek() {
		case ' ', '\t', '\r', '\n':
			p.take()
		default:
			return
		}
	}
}
func (p *codexStream) expect(c byte) {
	if p.take() != c {
		p.bad = true
	}
}

// stringValue validates every escape and captures a bounded key or timestamp.
// A timestamp may have arbitrarily long leading and trailing Unicode whitespace.
// mode 0 validates only, 1 captures a key, 2 captures a trimmed timestamp.
func (p *codexStream) stringValue(mode int, keys []string) string {
	p.expect('"')
	var b [128]byte
	n := 0
	overflow := false
	started := false
	spaceAfter := false
	put := func(r rune) {
		if mode == 0 || overflow {
			return
		}
		if mode == 2 && unicode.IsSpace(r) {
			if started {
				spaceAfter = true
			}
			return
		}
		if mode == 2 {
			if spaceAfter {
				overflow = true
				return
			}
			started = true
		}
		if n+utf8.RuneLen(r) > len(b) || (mode == 2 && n+utf8.RuneLen(r) > 42) {
			overflow = true
		}
		if !overflow {
			n += utf8.EncodeRune(b[n:], r)
		}
	}
	for !p.bad {
		c := p.peek()
		if c == 0 {
			p.bad = true
			break
		}
		if c == '"' {
			p.take()
			break
		}
		if c < 0x20 {
			p.bad = true
			break
		}
		if c != '\\' {
			r, _, e := p.r.ReadRune()
			if e != nil {
				if e != io.EOF {
					p.err = e
				}
				p.bad = true
				break
			}
			put(r)
			continue
		}
		p.take()
		e := p.take()
		switch e {
		case '"', '\\', '/':
			put(rune(e))
		case 'b':
			put('\b')
		case 'f':
			put('\f')
		case 'n':
			put('\n')
		case 'r':
			put('\r')
		case 't':
			put('\t')
		case 'u':
			u := p.hex4()
			if u >= 0xd800 && u < 0xdc00 {
				p.expect('\\')
				p.expect('u')
				lo := p.hex4()
				if lo < 0xdc00 || lo >= 0xe000 {
					p.bad = true
				} else {
					put(utf16.DecodeRune(rune(u), rune(lo)))
				}
			} else if u >= 0xdc00 && u < 0xe000 {
				p.bad = true
			} else {
				put(rune(u))
			}
		default:
			p.bad = true
		}
	}
	if overflow {
		return "\x00"
	}
	if mode == 1 {
		for _, k := range keys {
			if string(b[:n]) == k {
				return k
			}
		}
		return ""
	}
	if mode == 2 {
		return string(b[:n])
	}
	return ""
}
func (p *codexStream) hex4() rune {
	var n rune
	for i := 0; i < 4; i++ {
		c := p.take()
		n <<= 4
		switch {
		case c >= '0' && c <= '9':
			n += rune(c - '0')
		case c >= 'a' && c <= 'f':
			n += rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			n += rune(c - 'A' + 10)
		default:
			p.bad = true
		}
	}
	return n
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// streamNumber retains the state needed by Go's decimal parser: its first 800
// significant digits, discarded-nonzero flag, and both decimal-point counters.
// Its fast and fallback paths differ for enormous integer mantissas.
func (p *codexStream) number(capture bool) core.JSON {
	neg := false
	if p.peek() == '-' {
		p.take()
		neg = true
	}
	var digits [800]byte
	nd, total, dpFast, dpSlow := 0, 0, 0, 0
	sticky := false
	seen := false
	dot := false
	digit := func(c byte) {
		if !seen && c == '0' {
			dpFast--
			dpSlow--
			return
		}
		seen = true
		total++
		if nd < len(digits) {
			digits[nd] = c
			nd++
		} else if c != '0' {
			sticky = true
		}
	}
	if p.peek() == '0' {
		digit(p.take())
	} else if p.peek() >= '1' && p.peek() <= '9' {
		for isDigit(p.peek()) {
			digit(p.take())
		}
	} else {
		p.bad = true
		return core.JNull
	}
	if p.peek() == '.' {
		dot = true
		dpFast = total
		dpSlow = nd
		p.take()
		if !isDigit(p.peek()) {
			p.bad = true
		}
		for isDigit(p.peek()) {
			digit(p.take())
		}
	}
	e, esign := 0, 1
	if p.peek() == 'e' || p.peek() == 'E' {
		p.take()
		if p.peek() == '+' || p.peek() == '-' {
			if p.take() == '-' {
				esign = -1
			}
		}
		if !isDigit(p.peek()) {
			p.bad = true
		}
		for isDigit(p.peek()) {
			c := p.take()
			if e < 10000 {
				e = e*10 + int(c-'0')
			}
		}
	}
	e *= esign
	if !capture {
		return core.JNull
	}
	if !seen {
		if neg {
			return core.JNum("-0")
		}
		return core.JNum("0")
	}
	// readFloat counts all significant digits; decimal.set caps its count at 800.
	if !dot {
		dpFast = total
		dpSlow = nd
	}
	dpFast += e
	dpSlow += e
	first := min(total, 19)
	mant := string(digits[:first])
	trunc := sticky
	for i := 19; i < nd; i++ {
		if digits[i] != '0' {
			trunc = true
			break
		}
	}
	sign := ""
	if neg {
		sign = "-"
	}
	parse := func(m string, power int) (float64, error) {
		return strconv.ParseFloat(sign+m+"e"+strconv.Itoa(power), 64)
	}
	f, err := parse(mant, dpFast-first)
	if trunc && dpFast-first <= 310 && dpFast-first >= -345 {
		m, _ := strconv.ParseUint(mant, 10, 64)
		f1, _ := parse(strconv.FormatUint(m+1, 10), dpFast-first)
		if f != f1 {
			// A bounded representative recreates decimal.set's first 800 digits,
			// its decimal point and its sticky bit; strconv performs the rounding.
			s := string(digits[:1]) + "." + string(digits[1:nd])
			if sticky {
				s += "1"
			}
			if nd == 1 && !sticky {
				s += "0"
			}
			f, err = parse(s, dpSlow-1)
		}
	}
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return core.JNull
	}
	return core.JNum(strconv.FormatFloat(f, 'g', -1, 64))
}

// value validates one JSON value and keeps only what a reading can use. scope picks the
// object being read: 1 root, 2 payload, 3 limits, 4 a window; 5 is the timestamp string
// and 6 a window's number (the only numbers captured); 0 validates and discards.
//
// The keys listed per scope below are exactly the fields ParseCodexLine and
// CodexPrimaryReset read. If either starts to read another field, add it here too, or
// lines over 64 KiB (the only ones that come through this parser) would stop matching
// what shorter lines give. TestOversizedCodexMatchesLegacy compares the two paths.
func (p *codexStream) value(scope, depth int) core.JSON {
	p.ws()
	c := p.peek()
	switch c {
	case '{':
		if depth >= 64 {
			p.bad = true
			return core.JNull
		}
		p.take()
		var keys []string
		switch scope {
		case 1:
			keys = []string{"timestamp", "payload"}
		case 2:
			keys = []string{"rate_limits"}
		case 3:
			keys = []string{"primary", "secondary"}
		case 4:
			keys = []string{"used_percent", "resets_at", "resets_in_seconds", "window_minutes"}
		}
		var vals [4]core.JSON
		var present [4]bool
		p.ws()
		if p.peek() == '}' {
			p.take()
			return core.JObj()
		}
		for !p.bad {
			p.ws()
			if p.peek() != '"' {
				p.bad = true
				break
			}
			mode := 0
			if len(keys) > 0 {
				mode = 1
			}
			key := p.stringValue(mode, keys)
			p.ws()
			p.expect(':')
			child := 0
			idx := -1
			for i, k := range keys {
				if key == k {
					idx = i
					if key == "timestamp" {
						child = 5
					} else if scope == 4 {
						child = 6
					} else {
						child = scope + 1
					}
					break
				}
			}
			v := p.value(child, depth+1)
			if idx >= 0 {
				vals[idx] = v
				present[idx] = true
			}
			p.ws()
			c = p.take()
			if c == '}' {
				break
			}
			if c != ',' {
				p.bad = true
				break
			}
		}
		if scope == 0 {
			return core.JNull
		}
		props := make([]core.Prop, 0, len(keys))
		for i, k := range keys {
			if present[i] {
				props = append(props, core.P(k, vals[i]))
			}
		}
		return core.JObj(props...)
	case '[':
		if depth >= 64 {
			p.bad = true
			return core.JNull
		}
		p.take()
		p.ws()
		if p.peek() == ']' {
			p.take()
			return core.JArr()
		}
		for !p.bad {
			p.value(0, depth+1)
			p.ws()
			c = p.take()
			if c == ']' {
				break
			}
			if c != ',' {
				p.bad = true
				break
			}
		}
		return core.JArr()
	case '"':
		mode := 0
		if scope == 5 {
			mode = 2
		}
		return core.JStr(p.stringValue(mode, nil))
	case 't':
		for _, c := range "true" {
			p.expect(byte(c))
		}
		return core.JBool(true)
	case 'f':
		for _, c := range "false" {
			p.expect(byte(c))
		}
		return core.JBool(false)
	case 'n':
		for _, c := range "null" {
			p.expect(byte(c))
		}
		return core.JNull
	default:
		if c == '-' || isDigit(c) {
			return p.number(scope == 6)
		}
		p.bad = true
		return core.JNull
	}
}

func oversizedCodex(r io.ReaderAt, start, end int64, encoding int, first bool, now time.Time, buf []byte) (Reading, bool, error) {
	// Reject irrelevant lines before constructing a parser. The caller reuses the
	// scratch buffer across lines, keeping allocations bounded on long rollouts.
	needle := codexLimitsKey
	if encoding != 0 {
		needle = make([]byte, 2*len(codexLimitsKey))
		for i, c := range codexLimitsKey {
			if encoding == 1 {
				needle[2*i] = c
			} else {
				needle[2*i+1] = c
			}
		}
	}
	found, overlap := false, 0
	for pos := start; pos < end; {
		n := int(min(int64(len(buf)-overlap), end-pos))
		got, err := r.ReadAt(buf[overlap:overlap+n], pos)
		if got != n {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Reading{}, false, err
		}
		pos += int64(n)
		n += overlap
		for i := 0; i <= n-len(needle); {
			j := bytes.Index(buf[i:n], needle)
			if j < 0 {
				break
			}
			i += j
			if encoding == 0 || (pos-int64(n)+int64(i)-start)%2 == 0 {
				found = true
				break
			}
			i++
		}
		if found {
			break
		}
		overlap = min(n, len(needle)-1)
		copy(buf[:overlap], buf[n-overlap:n])
	}
	if !found {
		return Reading{}, false, nil
	}
	section := &exactSection{r: r, pos: start, end: end}
	var input io.Reader = section
	if encoding != 0 {
		input = &utf16Reader{r: bufio.NewReader(section), le: encoding == 1}
	} else if first {
		var b [3]byte
		n, err := r.ReadAt(b[:], start)
		if n != len(b) {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Reading{}, false, err
		}
		if n == 3 && bytes.Equal(b[:], utf8BOM) {
			section = &exactSection{r: r, pos: start + 3, end: end}
			input = section
		}
	}
	// The prefilter is over decoded text, not over decoded JSON keys.
	p := &codexStream{r: bufio.NewReaderSize(input, 32<<10)}
	root := p.value(1, 0)
	p.ws()
	if b, err := p.r.Peek(1); len(b) != 0 {
		p.bad = true
	} else if err != nil && err != io.EOF {
		p.err = err
	}
	if p.err != nil {
		return Reading{}, false, p.err
	}
	if p.bad {
		return Reading{}, false, nil
	}
	q, ok := ParseCodexLine(root.Compact(), now)
	return q, ok, nil
}
