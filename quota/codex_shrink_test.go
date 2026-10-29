package quota

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

// shrinkFile is a rollout that Codex rewrites shorter between the scan's Stat and its reads:
// the first Stat reports the old size, while every read already sees the new bytes. The old
// whole-file read could not fail this way, because it read up to whatever end it found.
type shrinkFile struct {
	old, cur []byte
	stats    int
}

type shrinkInfo struct {
	os.FileInfo
	size int64
}

func (i shrinkInfo) Size() int64 { return i.size }

func (f *shrinkFile) Stat() (os.FileInfo, error) {
	f.stats++
	if f.stats == 1 {
		return shrinkInfo{size: int64(len(f.old))}, nil
	}
	return shrinkInfo{size: int64(len(f.cur))}, nil
}

func (f *shrinkFile) ReadAt(p []byte, off int64) (int, error) {
	return bytes.NewReader(f.cur).ReadAt(p, off)
}

func TestRolloutScanFileShrankMidScan(t *testing.T) {
	short := tailAccepted(10) + "\n"
	cases := map[string]struct{ old, cur string }{
		"rewritten shorter":        {short + tailAccepted(20) + "\n", short},
		"truncated to empty":       {short, ""},
		"shrank under a long line": {short + tailPadded(100<<10) + "\n", short},
	}
	for name, c := range cases {
		want, wantOK := oracleBytes([]byte(c.cur), tailNow)
		f := &shrinkFile{old: []byte(c.old), cur: []byte(c.cur)}
		got, ok, err := rolloutScan(f, tailNow)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if ok != wantOK || (ok && !reflect.DeepEqual(got, want)) {
			t.Fatalf("%s: got %s, want %s", name, tailShow(got, ok), tailShow(want, wantOK))
		}
	}
}
