package quota

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-quota/internal/core"
)

func TestMain(m *testing.M) {
	ownedSupport, err := os.MkdirTemp("", "agent-quota-quota-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Setenv("AGENTQUOTA_DATA_DIR", ownedSupport); err != nil {
		_ = os.RemoveAll(ownedSupport)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(ownedSupport)
	os.Exit(code)
}

func snapshotInput(session string, used float64, fiveReset, weekReset int64, duration string) []byte {
	return []byte(fmt.Sprintf(`{"session_id":%q,"rate_limits":{"five_hour":{"used_percentage":%g,"resets_at":%d},"seven_day":{"used_percentage":%g,"resets_at":%d}},"cost":{"total_api_duration_ms":%s},"transcript":"never persist this","account":{"secret":"never persist this either"}}`, session, used, fiveReset, used/2, weekReset, duration))
}

func TestClaudeSnapshotCaptureDeduplicatesAndChoosesNewestSession(t *testing.T) {
	base := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	claudeHome := filepath.Join(t.TempDir(), "claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	if err := os.MkdirAll(claudeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), []byte(`{"invalid":"credential"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reset := base.Add(time.Hour).Unix()
	weekReset := base.Add(48 * time.Hour).Unix()
	payload := snapshotInput("session-a", 37, reset, weekReset, "1000")
	if !CaptureClaudeSnapshot(payload, base) {
		t.Fatal("first valid snapshot was not stored")
	}
	if CaptureClaudeSnapshot(payload, base.Add(time.Minute)) {
		t.Fatal("identical timer callback advanced the snapshot")
	}
	if got, ok := claudeSnapshotReading(base.Add(time.Minute)); !ok || *got.Used != 37 || !strings.Contains(got.Detail, "as of "+FormatLocal(base, "HH:mm")) {
		t.Fatalf("fresh snapshot = %#v, %v", got, ok)
	}
	if got := Claude(base.Add(time.Minute)); !got.OK() || *got.Used != 37 {
		t.Fatalf("public Claude read did not prefer the local snapshot: %#v", got)
	}
	root := filepath.Join(core.Support(), "claude-snapshots")
	var stored string
	fileCount := 0
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
			b, _ := os.ReadFile(path)
			stored += string(b)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if fileCount == 0 || !strings.Contains(stored, `"used":37`) || !strings.Contains(stored, `"observed_at"`) {
		t.Fatalf("test did not inspect an actual persisted snapshot: files=%d data=%q", fileCount, stored)
	}
	for _, secret := range []string{"transcript", "account", "never persist", "session-a"} {
		if strings.Contains(stored, secret) {
			t.Fatalf("stored snapshot contains %q", secret)
		}
	}
	if CaptureClaudeSnapshot(snapshotInput("session-a", 37, reset, weekReset, "2000"), base.Add(2*time.Minute)) != true {
		t.Fatal("changed actual-response counter did not advance snapshot")
	}
	if !CaptureClaudeSnapshot(snapshotInput("session-b", 91, reset, weekReset, "3000"), base.Add(3*time.Minute)) {
		t.Fatal("second session snapshot was not stored")
	}
	got, ok := claudeSnapshotReading(base.Add(3 * time.Minute))
	if !ok || *got.Used != 91 {
		t.Fatalf("newest session reading = %#v, %v", got, ok)
	}
	path := snapshotPath(claudeHash(claudeScope()), claudeHash("session-b"))
	s, err := readClaudeSnapshot(path)
	if err != nil || s.ObservedAt != base.Add(3*time.Minute).UnixNano() {
		t.Fatalf("stored time = %d, %v; err=%v", s.ObservedAt, base.Add(3*time.Minute).UnixNano(), err)
	}
}

func TestClaudeSnapshotRejectsInvalidAndUsesFreshMatchingScopeOnly(t *testing.T) {
	base := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	home := filepath.Join(t.TempDir(), "home-a")
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	reset := base.Add(time.Minute).Unix()
	if CaptureClaudeSnapshot([]byte(`{}`), base) || CaptureClaudeSnapshot(nil, base) {
		t.Fatal("invalid input was accepted")
	}
	if !CaptureClaudeSnapshot(snapshotInput("session", 40, reset, reset, "1"), base) {
		t.Fatal("valid input was rejected")
	}
	if CaptureClaudeSnapshot(snapshotInput("session", 150, reset, reset, "2"), base.Add(time.Minute)) {
		t.Fatal("invalid quota overwrote valid snapshot")
	}
	if _, ok := claudeSnapshotReading(base.Add(6 * time.Minute)); ok {
		t.Fatal("stale snapshot was used")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "home-b"))
	if _, ok := claudeSnapshotReading(base.Add(time.Minute)); ok {
		t.Fatal("snapshot from another Claude config scope was used")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	if _, ok := claudeSnapshotReading(base.Add(10 * time.Minute)); ok {
		t.Fatal("snapshot with expired windows was used")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "future-home"))
	future := base.Add(24 * time.Hour)
	futureReset := future.Add(time.Hour).Unix()
	if !CaptureClaudeSnapshot(snapshotInput("future", 10, futureReset, futureReset, "3"), future) {
		t.Fatal("future-clock sample was not stored")
	}
	if _, ok := claudeSnapshotReading(base); ok {
		t.Fatal("far-future observation was accepted")
	}
}

func TestClaudeSnapshotCachePrunesOldSessionsAndKeepsForeignFiles(t *testing.T) {
	base := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	home := filepath.Join(t.TempDir(), "many-sessions")
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	scopeHash := claudeHash(claudeScope())
	scopeDir := filepath.Join(core.Support(), "claude-snapshots", scopeHash)
	if err := os.MkdirAll(scopeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(scopeDir, "foreign.json")
	temp := filepath.Join(scopeDir, ".capture-keep.tmp")
	if err := os.WriteFile(foreign, []byte("keep foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temp, []byte("keep temp"), 0o600); err != nil {
		t.Fatal(err)
	}
	reset := base.Add(time.Hour).Unix()
	for i := range 129 {
		used := float64(i%99 + 1)
		if i == 128 {
			used = 99
		}
		now := base.Add(time.Duration(i) * time.Second)
		input := snapshotInput(fmt.Sprintf("synthetic-session-%03d", i), used, reset, reset, fmt.Sprint(i+1))
		if !CaptureClaudeSnapshot(input, now) {
			t.Fatalf("snapshot %d was not stored", i)
		}
	}
	entries, err := os.ReadDir(scopeDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if strictClaudeSnapshotName(entry.Name()) {
			count++
		}
	}
	if count != claudeSnapshotMaxFiles {
		t.Fatalf("cache has %d snapshots; want %d", count, claudeSnapshotMaxFiles)
	}
	for path, want := range map[string]string{foreign: "keep foreign", temp: "keep temp"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("foreign file %s changed: %q, %v", path, got, err)
		}
	}
	reading, ok := claudeSnapshotReading(base.Add(128 * time.Second))
	if !ok || *reading.Used != 99 {
		t.Fatalf("newest retained snapshot = %#v, %v", reading, ok)
	}
}

func TestPollerRefreshOneReadsOnlyRequestedEnabledProvider(t *testing.T) {
	var claude, other atomic.Int32
	release := make(chan struct{})
	on := func(id string) bool { return id == ItemClaude || id == ItemKiro }
	p := NewPoller(func(id string) Reading {
		if id == ItemClaude {
			claude.Add(1)
			<-release
		} else {
			other.Add(1)
		}
		return newReading(1, "ok")
	}, on, func() {})
	p.book.readings[ItemKiro] = stamped{newReading(25, "kept"), time.Now()}
	p.book.good[ItemKiro] = p.book.readings[ItemKiro]
	p.book.busy[ItemKiro] = true
	p.RefreshOne(ItemClaude)
	p.RefreshOne(ItemClaude)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		finished := !p.book.Busy(ItemClaude)
		p.mu.Unlock()
		if claude.Load() == 1 && !finished {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if claude.Load() != 1 || other.Load() != 0 {
		t.Fatalf("reader counts Claude=%d other=%d", claude.Load(), other.Load())
	}
	close(release)
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		finished := !p.book.Busy(ItemClaude)
		p.mu.Unlock()
		if finished {
			break
		}
		time.Sleep(time.Millisecond)
	}
	p.mu.Lock()
	if !p.book.Busy(ItemKiro) || p.book.readings[ItemKiro].r.Detail != "kept" || p.book.good[ItemKiro].r.Used == nil {
		p.mu.Unlock()
		t.Fatal("refreshing Claude changed Kiro state")
	}
	p.mu.Unlock()
}
