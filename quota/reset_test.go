package quota

// Task 1 reset tails: the primary/five-hour reset instants behind the Detail
// strings. CodexPrimaryReset/ClaudeFiveHourReset expose the instant so later
// work (pill tooltips, "resets in …") can share one parse instead of copying it.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexPrimaryReset(t *testing.T) {
	// A token_count line carrying resets_at: the exact unix instant comes back.
	want := now().Add(2 * time.Hour).Unix()
	got, ok := CodexPrimaryReset(codexLine(want), now())
	if !ok || got != want {
		t.Fatalf("CodexPrimaryReset = (%v, %v), want (%v, true)", got, ok, want)
	}
	// An expired reset counts as empty, same rule as ParseCodexLine.
	if v, ok := CodexPrimaryReset(codexLine(now().Add(-time.Hour).Unix()), now()); ok {
		t.Errorf("expired reset read as %v", v)
	}
	// No primary window: no reset to return.
	for _, line := range []string{
		`{"payload":{"rate_limits":null}}`,
		`{"payload":{}}`,
		"not json",
	} {
		if v, ok := CodexPrimaryReset(line, now()); ok {
			t.Errorf("%s: read reset %v", line, v)
		}
	}
}

func TestClaudeFiveHourReset(t *testing.T) {
	// The five_hour instant wins even when the seven_day window is fuller/later,
	// so callers never mix the two windows up.
	fiveReset := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	sevenReset := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	body := fmt.Sprintf(`{"five_hour":{"utilization":18.0,"resets_at":%q},"seven_day":{"utilization":46.0,"resets_at":%q}}`,
		fiveReset.Format(time.RFC3339), sevenReset.Format(time.RFC3339))
	got, ok := ClaudeFiveHourReset(body)
	if !ok || got != fiveReset.Unix() {
		t.Fatalf("ClaudeFiveHourReset = (%v, %v), want (%v, true)", got, ok, fiveReset.Unix())
	}
	// Missing or expired five_hour reset: no instant to return.
	if v, ok := ClaudeFiveHourReset(`{"seven_day":{"utilization":1}}`); ok {
		t.Errorf("missing five_hour read as %v", v)
	}
	expired := fmt.Sprintf(`{"five_hour":{"utilization":90,"resets_at":%q}}`,
		time.Now().UTC().Add(-5*time.Minute).Format(time.RFC3339))
	if v, ok := ClaudeFiveHourReset(expired); ok {
		t.Errorf("expired five_hour read as %v", v)
	}
}

func TestParseCodexLineResetTail(t *testing.T) {
	// A line WITH a primary reset gains the local HH:mm tail.
	resetUnix := now().Add(2 * time.Hour).Unix()
	q, ok := ParseCodexLine(codexLine(resetUnix), now())
	if !ok {
		t.Fatal("no reading")
	}
	resetLocal := FormatLocal(time.Unix(resetUnix, 0).UTC(), "HH:mm")
	want := "5h 38% · week 12% · as of 26 Sep 11:55 · resets " + resetLocal
	if q.Detail != want {
		t.Errorf("detail with reset:\n got %q\nwant %q", q.Detail, want)
	}
	// A line WITHOUT any usable reset keeps today's Detail byte-for-byte.
	plain, ok := ParseCodexLine(`{"payload":{"rate_limits":{"primary":{"used_percent":1,"window_minutes":150},"secondary":{"used_percent":2,"window_minutes":45.5}}}}`, now())
	if !ok {
		t.Fatal("no reading for reset-less line")
	}
	wantPlain := "2h 1% · 45.5m 2% · as of " + FormatLocal(now(), "d MMM HH:mm")
	if plain.Detail != wantPlain {
		t.Errorf("detail without reset:\n got %q\nwant %q", plain.Detail, wantPlain)
	}
	if strings.Contains(plain.Detail, "resets") {
		t.Errorf("reset-less detail gained a tail: %q", plain.Detail)
	}
}

func TestParseCodexLineCarriesTheFiveHourReset(t *testing.T) {
	// A primary window labelled 5h hands its reset to Reset5h as a real instant,
	// so the UI can count down instead of re-parsing the Detail text.
	want := now().Add(2 * time.Hour)
	q, ok := ParseCodexLine(codexLine(want.Unix()), now())
	if !ok {
		t.Fatal("no reading")
	}
	if !q.Reset5h.Equal(time.Unix(want.Unix(), 0)) {
		t.Fatalf("Reset5h = %v, want %v", q.Reset5h, want)
	}
	// A missing window_minutes falls back to the "5h" label, so it still counts.
	line := fmt.Sprintf(`{"payload":{"rate_limits":{"primary":{"used_percent":9,"resets_at":%d}}}}`, want.Unix())
	q, ok = ParseCodexLine(line, now())
	if !ok || !q.Reset5h.Equal(time.Unix(want.Unix(), 0)) {
		t.Fatalf("missing window_minutes: ok=%v Reset5h=%v", ok, q.Reset5h)
	}
	// A primary labelled anything else ("2h" here) is a different window, even
	// when it carries a future reset; the weekly secondary's never counts.
	line = fmt.Sprintf(`{"payload":{"rate_limits":{"primary":{"used_percent":1,"window_minutes":150,"resets_at":%d},"secondary":{"used_percent":2,"window_minutes":10080,"resets_at":%d}}}}`,
		want.Unix(), now().Add(48*time.Hour).Unix())
	q, ok = ParseCodexLine(line, now())
	if !ok {
		t.Fatal("no reading for the 2h primary")
	}
	if !q.Reset5h.IsZero() {
		t.Fatalf("2h primary set Reset5h = %v", q.Reset5h)
	}
	// An expired primary reset counts as none, matching the empty-window rule.
	q, ok = ParseCodexLine(codexLine(now().Add(-time.Hour).Unix()), now())
	if !ok {
		t.Fatal("no reading for the expired reset")
	}
	if !q.Reset5h.IsZero() {
		t.Fatalf("expired reset set Reset5h = %v", q.Reset5h)
	}
}

func TestParseClaudeUsageCarriesTheFiveHourReset(t *testing.T) {
	// The five_hour instant is carried even when the weekly window is the top
	// one: Detail keeps the top window's reset, Reset5h is always the 5h one.
	five := now().Add(2 * time.Hour)
	week := now().Add(72 * time.Hour)
	body := fmt.Sprintf(`{"five_hour":{"utilization":18.0,"resets_at":%q},"seven_day":{"utilization":46.0,"resets_at":%q}}`,
		iso(five), iso(week))
	r := ParseClaudeUsage(body, nil, now())
	wantUsed(t, r, 46)
	if !r.Reset5h.Equal(five) {
		t.Fatalf("week-heavier Reset5h = %v, want %v", r.Reset5h, five)
	}
	// An already-passed five_hour reset is none, even though it still empties
	// the window for the percentage.
	gone := iso(now().Add(-5 * time.Minute))
	r = ParseClaudeUsage(fmt.Sprintf(`{"five_hour":{"utilization":90,"resets_at":%q},"seven_day":{"utilization":12,"resets_at":null}}`, gone), nil, now())
	wantUsed(t, r, 12)
	if !r.Reset5h.IsZero() {
		t.Fatalf("expired five_hour set Reset5h = %v", r.Reset5h)
	}
}

func TestAntigravityCarriesTheEarliestFiveHourReset(t *testing.T) {
	base := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	// The earliest future reset among the "5h" buckets wins; a weekly bucket
	// whose reset lands sooner is a different window and is ignored.
	earlier := base.Add(2 * time.Hour)
	later := base.Add(3 * time.Hour)
	weekly := base.Add(time.Hour)
	input := fmt.Sprintf(`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[`+
		`{"id":"week","window":"weekly","remaining_fraction":0.9,"reset_time":%q},`+
		`{"id":"later5h","window":"5h","remaining_fraction":0.5,"reset_time":%q},`+
		`{"id":"earlier5h","window":"5h","remaining_fraction":0.4,"reset_time":%q}]}]}}}`,
		weekly.Format(time.RFC3339), later.Format(time.RFC3339), earlier.Format(time.RFC3339))
	r := ParseAntigravity(input, base)
	if !r.OK() {
		t.Fatalf("no reading: %#v", r)
	}
	if !r.Reset5h.Equal(earlier) {
		t.Fatalf("Reset5h = %v, want %v", r.Reset5h, earlier)
	}
	// With only an expired 5h reset there is none, whatever the weekly says.
	expired := fmt.Sprintf(`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[`+
		`{"id":"week","window":"weekly","remaining_fraction":0.9,"reset_time":%q},`+
		`{"id":"old5h","window":"5h","remaining_fraction":0.5,"reset_time":%q}]}]}}}`,
		weekly.Format(time.RFC3339), base.Add(-time.Hour).Format(time.RFC3339))
	r = ParseAntigravity(expired, base)
	if !r.OK() {
		t.Fatalf("no reading: %#v", r)
	}
	if !r.Reset5h.IsZero() {
		t.Fatalf("expired 5h bucket set Reset5h = %v", r.Reset5h)
	}
}

func TestClaudeSnapshotReadingCarriesTheFiveHourReset(t *testing.T) {
	base := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	claudeHome := filepath.Join(t.TempDir(), "claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", claudeHome)
	if err := os.MkdirAll(claudeHome, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), []byte(`{"invalid":"credential"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fiveReset := base.Add(2 * time.Hour).Unix()
	weekReset := base.Add(48 * time.Hour).Unix()
	// A week-heavier snapshot still carries the five-hour instant, matching the
	// OAuth parser: Detail shows the top window's reset, Reset5h the 5h one.
	input := fmt.Sprintf(`{"session_id":"s","rate_limits":{"five_hour":{"used_percentage":10,"resets_at":%d},"seven_day":{"used_percentage":80,"resets_at":%d}}}`,
		fiveReset, weekReset)
	if !CaptureClaudeSnapshot([]byte(input), base) {
		t.Fatal("snapshot was not stored")
	}
	got, ok := claudeSnapshotReading(base.Add(time.Minute))
	if !ok {
		t.Fatal("no snapshot reading")
	}
	wantUsed(t, got, 80)
	if !got.Reset5h.Equal(time.Unix(fiveReset, 0)) {
		t.Fatalf("Reset5h = %v, want %v", got.Reset5h, time.Unix(fiveReset, 0))
	}
}
