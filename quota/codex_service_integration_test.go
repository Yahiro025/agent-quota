package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the service wire format with the actual Codex reader and isolated
// rollout data, without launching Bloom or touching other providers.
func TestCodexServiceRefreshMatchesStandalone(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-test.jsonl")
	write := func(used float64) {
		t.Helper()
		if err := os.WriteFile(path, []byte(tailAccepted(used)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	changed := make(chan struct{}, 4)
	p := NewPoller(func(id string) Reading { return CodexIn(home, tailNow) }, func(id string) bool { return id == ItemCodex }, func() { changed <- struct{}{} })
	s := NewService(p)
	for _, used := range []float64{17, 41} {
		write(used)
		if events, done := s.Handle(`{"v":1,"op":"refresh","id":"codex"}`); done || len(events) != 0 {
			t.Fatalf("refresh: %v, %v", events, done)
		}
		select {
		case <-changed:
		case <-time.After(5 * time.Second):
			t.Fatal("refresh timed out")
		}
		want := CodexIn(home, tailNow)
		line, err := EncodeEvent(s.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		var event ServiceEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		got := findAgent(t, event, ItemCodex)
		if event.Op != "snapshot" || got.State != "ok" || got.Used == nil || want.Used == nil || *got.Used != *want.Used || *got.Used != used || !got.Reset5h.Equal(want.Reset5h) || !got.Reset5h.Equal(tailNow.Add(2*time.Hour)) {
			t.Fatalf("wire reading %+v; standalone %+v", got, want)
		}
	}
}
