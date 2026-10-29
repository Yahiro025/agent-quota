package quota

// Finding a Codex rollout's newest rate-limits line without reading the file whole.
// Rollouts reach hundreds of MB (one here is 380 MB) and only the last accepted line
// matters, so the scan walks backwards from the end in fixed windows. Scanner buffers
// are bounded; parsing each candidate also allocates. Reading the file whole allocated
// about three times its size on every 30-second poll.

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"agent-quota/internal/core"
)

const (
	// codexTailWindow is how much each backward step reads. The newest line is usually
	// under 10 KB from the end, so one window almost always settles it.
	codexTailWindow = 256 << 10
	// codexLineCap is the longest line the scan holds. Real rate-limit lines stay under
	// 1 KB, while other lines (tool output) reach tens of MB, and parsing costs a few
	// hundred times the line's length. Larger lines use a streaming projection.
	codexLineCap = 64 << 10
)

var (
	codexLimitsKey = []byte(`"rate_limits"`)
	utf8BOM        = []byte{0xEF, 0xBB, 0xBF}
)

// lastRateLimits is the last line, in file order, that holds "rate_limits" and that
// ParseCodexLine accepts. Lines end at \n or \r, as the whole-file read split them, and
// the unterminated last line counts. Large lines use bounded streaming validation.
// size is read once, so bytes appended meanwhile are ignored.
func lastRateLimits(r io.ReaderAt, size int64, now time.Time, window, lineCap int) (Reading, bool, error) {
	if size <= 0 || window <= 0 {
		return Reading{}, false, nil
	}
	// No line is longer than the file, so this keeps the line buffer small for small files.
	limit := int(min(int64(lineCap), size))
	buf := make([]byte, int(min(int64(window), size)))
	// store keeps the back of a line whose front lies in an earlier window, in
	// store[start:]. It fills from the right, so the next piece goes in front without
	// moving what is there and a finished line is one slice. The first line that needs
	// it allocates it.
	var store []byte
	start := limit
	skipping := false // the line is too large for the fast-path buffer
	lineEnd := size
	var scratch []byte
	large := func(start, end int64, first bool) (Reading, bool, error) {
		if scratch == nil {
			scratch = make([]byte, 32<<10)
		}
		return oversizedCodex(r, start, end, 0, first, now, scratch)
	}

	accept := func(line []byte, first bool) (Reading, bool) {
		if !bytes.Contains(line, codexLimitsKey) {
			return Reading{}, false
		}
		if first {
			// ReadAllText drops a UTF-8 BOM; only the first line can carry one.
			line = bytes.TrimPrefix(line, utf8BOM)
		}
		return ParseCodexLine(core.Lossy(line), now)
	}
	// whole is a line's front piece followed by the stored back, or false when that is
	// over the cap. Without a stored back the front is used where it lies.
	whole := func(front []byte) ([]byte, bool) {
		if len(front) > start {
			return nil, false
		}
		if start == limit {
			return front, true
		}
		copy(store[start-len(front):], front)
		return store[start-len(front):], true
	}

	for pos := size; pos > 0; {
		n := int(min(int64(len(buf)), pos))
		pos -= int64(n)
		chunk := buf[:n]
		if got, err := r.ReadAt(chunk, pos); got < n {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Reading{}, false, err
		}
		end := n // chunk[end:] already belongs to a finished line or to the stored back
		for i := n - 1; i >= 0; i-- {
			if chunk[i] != '\n' && chunk[i] != '\r' {
				continue
			}
			// The line begins after this terminator; chunk[i+1:end] is its front.
			if !skipping {
				if line, ok := whole(chunk[i+1 : end]); ok {
					if q, ok := accept(line, false); ok {
						return q, true, nil
					}
				} else {
					skipping = true
				}
			}
			if skipping {
				q, ok, err := large(pos+int64(i)+1, lineEnd, false)
				if err != nil || ok {
					return q, ok, err
				}
			}
			lineEnd = pos + int64(i)
			skipping, start, end = false, limit, i
		}
		// chunk[:end] is the back of a line that begins in an earlier window.
		if end > 0 && !skipping {
			if end > start {
				skipping, start = true, limit
			} else {
				if store == nil {
					store = make([]byte, limit)
				}
				start -= end
				copy(store[start:], chunk[:end])
			}
		}
	}
	// What is stored now is the line that begins at offset 0.
	if !skipping && start < limit {
		if q, ok := accept(store[start:], true); ok {
			return q, true, nil
		}
	}
	if skipping {
		return large(0, lineEnd, true)
	}
	return Reading{}, false, nil
}

// rolloutLimits reads one rollout file's newest accepted rate-limits line.
func rolloutLimits(path string, now time.Time) (Reading, bool, error) {
	// Codex may be writing to it right now; core.Open shares the file on both sides.
	f, err := core.Open(path)
	if err != nil {
		return Reading{}, false, err
	}
	defer f.Close()
	return rolloutScan(f, now)
}

// rolloutFile is an open rollout: random access to its bytes and its current size.
type rolloutFile interface {
	io.ReaderAt
	Stat() (os.FileInfo, error)
}

// codexScanTries bounds the rescans after a rollout got shorter mid-scan.
const codexScanTries = 3

// rolloutScan scans f, starting over at the new size when f got shorter during a scan.
// Reads past the size taken at the start then come up short. The old whole-file read
// stopped at whatever end it found and never failed this way, so a rewrite while Codex
// works must not show Codex as failed either.
func rolloutScan(f rolloutFile, now time.Time) (Reading, bool, error) {
	var err error
	for range codexScanTries {
		var q Reading
		var ok bool
		if q, ok, err = rolloutScanOnce(f, now); !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return q, ok, err
		}
	}
	return Reading{}, false, err
}

// rolloutScanOnce is one scan of f at the size it has now.
func rolloutScanOnce(f rolloutFile, now time.Time) (Reading, bool, error) {
	st, err := f.Stat()
	if err != nil {
		return Reading{}, false, err
	}
	if st.Size() >= 2 {
		var bom [2]byte
		if _, err := f.ReadAt(bom[:], 0); err != nil {
			return Reading{}, false, err
		}
		if bom == [2]byte{0xff, 0xfe} || bom == [2]byte{0xfe, 0xff} {
			return lastRateLimitsUTF16(f, st.Size(), now, bom)
		}
	}
	return lastRateLimits(f, st.Size(), now, codexTailWindow, codexLineCap)
}

// UTF-16 delimiters must be found on code-unit boundaries. Decode only candidate
// lines, using TextOf so malformed surrogates match the legacy file decoder.
func lastRateLimitsUTF16(r io.ReaderAt, size int64, now time.Time, bom [2]byte) (Reading, bool, error) {
	end := size - (size-2)%2 // TextOf discards an incomplete final code unit.
	buf := make([]byte, codexTailWindow)
	line := make([]byte, codexLineCap+2)
	copy(line, bom[:])
	needle := make([]byte, 2*len(codexLimitsKey))
	for i, c := range codexLimitsKey {
		if bom[0] == 0xff {
			needle[2*i] = c
		} else {
			needle[2*i+1] = c
		}
	}
	start := len(line)
	skipping := false
	lineEnd := end
	var scratch []byte
	large := func(start, end int64) (Reading, bool, error) {
		if scratch == nil {
			scratch = make([]byte, 32<<10)
		}
		encoding := 1
		if bom[0] == 0xfe {
			encoding = 2
		}
		return oversizedCodex(r, start, end, encoding, false, now, scratch)
	}
	for pos := end; pos > 2; {
		n := int(min(int64(len(buf)), pos-2))
		pos -= int64(n)
		chunk := buf[:n]
		if got, err := r.ReadAt(chunk, pos); got != n {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return Reading{}, false, err
		}
		for i := n - 2; i >= 0; i -= 2 {
			c := uint16(chunk[i]) | uint16(chunk[i+1])<<8
			if bom[0] == 0xfe {
				c = uint16(chunk[i])<<8 | uint16(chunk[i+1])
			}
			if c == '\r' || c == '\n' {
				if !skipping && start < len(line) && bytes.Contains(line[start:], needle) {
					copy(line[start-2:start], bom[:])
					text := core.TextOf(line[start-2:])
					if strings.Contains(text, `"rate_limits"`) {
						if q, ok := ParseCodexLine(text, now); ok {
							return q, true, nil
						}
					}
				}
				if skipping {
					q, ok, err := large(pos+int64(i)+2, lineEnd)
					if err != nil || ok {
						return q, ok, err
					}
				}
				lineEnd = pos + int64(i)
				start, skipping = len(line), false
			} else if !skipping {
				if start <= 2 {
					skipping = true
				} else {
					start -= 2
					copy(line[start:start+2], chunk[i:i+2])
				}
			}
		}
	}
	if !skipping && start < len(line) && bytes.Contains(line[start:], needle) {
		copy(line[start-2:start], bom[:])
		text := core.TextOf(line[start-2:])
		if strings.Contains(text, `"rate_limits"`) {
			q, ok := ParseCodexLine(text, now)
			return q, ok, nil
		}
	}
	if skipping {
		return large(2, lineEnd)
	}
	return Reading{}, false, nil
}
