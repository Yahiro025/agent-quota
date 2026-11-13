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

func TestClaudeFallsBackToCachedUsageUtilizationInClaudeJson(t *testing.T) {
	base := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	claudeHome := filepath.Join(t.TempDir(), "claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	if err := os.MkdirAll(claudeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	// Exact state when Claude Code invalid_grant clears accessToken/refreshToken.
	clearedCreds := `{"claudeAiOauth":{"accessToken":"","refreshToken":"","expiresAt":0,"subscriptionType":"pro"}}`
	if err := os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), []byte(clearedCreds), 0o600); err != nil {
		t.Fatal(err)
	}
	fetchedAt := base.Add(-2 * time.Hour)
	expiredFive := base.Add(-30 * time.Minute).Format(time.RFC3339Nano)
	activeWeek := base.Add(96 * time.Hour).Format(time.RFC3339Nano)
	claudeJSON := fmt.Sprintf(`{
		"oauthAccount":{"accountUuid":"acc-1","organizationType":"claude_pro"},
		"cachedUsageUtilization":{
			"fetchedAtMs":%d,
			"accountUuid":"acc-1",
			"utilization":{
				"five_hour":{"utilization":45,"resets_at":%q},
				"seven_day":{"utilization":2,"resets_at":%q}
			}
		}
	}`, fetchedAt.UnixMilli(), expiredFive, activeWeek)
	if err := os.WriteFile(filepath.Join(claudeHome, ".claude.json"), []byte(claudeJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Claude(base)
	if !got.OK() {
		t.Fatalf("Claude() failed despite cachedUsageUtilization in .claude.json: %#v", got)
	}
	if *got.Used != 2 {
		t.Fatalf("Used = %v, want 2 (expired 5h window should roll to 0): %#v", *got.Used, got)
	}
	if got.Plan != "Pro" {
		t.Fatalf("Plan = %q, want Pro", got.Plan)
	}
	if !got.ObservedAt.Equal(fetchedAt) {
		t.Fatalf("ObservedAt = %v, want %v", got.ObservedAt, fetchedAt)
	}
	if len(got.Windows) != 2 || got.Windows[0].Used != 0 || got.Windows[1].Used != 2 {
		t.Fatalf("Windows = %#v, want [5h:0, week:2]", got.Windows)
	}

	// Also works from backups/.claude.json.backup.* when .claude.json is absent.
	_ = os.Remove(filepath.Join(claudeHome, ".claude.json"))
	_ = os.Remove(filepath.Join(claudeHome, ".credentials.json"))
	backupDir := filepath.Join(claudeHome, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, ".claude.json.backup.1794149984830"), []byte(claudeJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	gotBackup := Claude(base)
	if !gotBackup.OK() || *gotBackup.Used != 2 || gotBackup.Plan != "Pro" {
		t.Fatalf("Claude() failed to read backup config: %#v", gotBackup)
	}
}

func TestClaudeFallsBackToPersistedSnapshotsAndChoosesNewestSource(t *testing.T) {
	base := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	claudeHome := filepath.Join(t.TempDir(), "claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	if err := os.MkdirAll(claudeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	expiredCreds := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"max"}}`, base.Add(-time.Hour).UnixMilli())
	if err := os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), []byte(expiredCreds), 0o600); err != nil {
		t.Fatal(err)
	}

	fiveReset := base.Add(time.Hour).Unix()
	weekReset := base.Add(48 * time.Hour).Unix()
	// Snapshot has 5h=44%, week=22% at base.
	if !CaptureClaudeSnapshot(snapshotInput("session-1", 44, fiveReset, weekReset, "100"), base) {
		t.Fatal("failed to capture initial snapshot")
	}

	// 2 hours later: snapshot is > 5m old and 5h window has expired, while week window (22%) is still active.
	later := base.Add(2 * time.Hour)
	got := Claude(later)
	if !got.OK() || *got.Used != 22 {
		t.Fatalf("Claude(later) = %#v, want OK with 22%% from weekly window after 5h reset passed", got)
	}
	if got.Plan != "Max" {
		t.Fatalf("Plan = %q, want Max", got.Plan)
	}
	if len(got.Windows) != 2 || got.Windows[0].Used != 0 || got.Windows[1].Used != 22 {
		t.Fatalf("Windows = %#v, want [5h:0, week:22]", got.Windows)
	}

	// Write a newer .claude.json cachedUsageUtilization at base + 90m (week = 55%).
	fetchedAt := base.Add(90 * time.Minute)
	claudeJSON := fmt.Sprintf(`{
		"cachedUsageUtilization":{
			"fetchedAtMs":%d,
			"utilization":{
				"five_hour":{"utilization":12,"resets_at":%q},
				"seven_day":{"utilization":55,"resets_at":%q}
			}
		}
	}`, fetchedAt.UnixMilli(), later.Add(time.Hour).Format(time.RFC3339Nano), later.Add(40*time.Hour).Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(claudeHome, ".claude.json"), []byte(claudeJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	gotNewerConfig := Claude(later)
	if !gotNewerConfig.OK() || *gotNewerConfig.Used != 55 {
		t.Fatalf("Claude(later) with newer .claude.json = %#v, want 55%%", gotNewerConfig)
	}

	// Capture an even newer snapshot at base + 110m (5h = 64%, week = 32%).
	if !CaptureClaudeSnapshot(snapshotInput("session-2", 64, later.Add(2*time.Hour).Unix(), weekReset, "200"), base.Add(110*time.Minute)) {
		t.Fatal("failed to capture newer snapshot")
	}
	gotNewerSnap := Claude(later)
	if !gotNewerSnap.OK() || *gotNewerSnap.Used != 64 {
		t.Fatalf("Claude(later) with newer snapshot = %#v, want 64%%", gotNewerSnap)
	}

	// Persisting an OAuth reading at base + 115m (5h = 78%, week = 40%) makes it the newest fallback.
	oauthReading := ParseClaudeUsage(
		fmt.Sprintf(`{"five_hour":{"utilization":78,"resets_at":%q},"seven_day":{"utilization":40,"resets_at":%q}}`,
			later.Add(3*time.Hour).Format(time.RFC3339Nano),
			later.Add(40*time.Hour).Format(time.RFC3339Nano)),
		sp("max"),
		base.Add(115*time.Minute),
	)
	if !recordClaudeReadingSnapshot(oauthReading, base.Add(115*time.Minute)) {
		t.Fatal("failed to record OAuth reading snapshot")
	}
	gotOAuthSnap := Claude(later)
	if !gotOAuthSnap.OK() || *gotOAuthSnap.Used != 78 {
		t.Fatalf("Claude(later) with recorded OAuth snapshot = %#v, want 78%%", gotOAuthSnap)
	}
}
