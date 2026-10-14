package quota

// Task 1 reset tails: the primary/five-hour reset instants behind the Detail
// strings. CodexPrimaryReset/ClaudeFiveHourReset expose the instant so later
// work (pill tooltips, "resets in …") can share one parse instead of copying it.

import (
	"fmt"
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
