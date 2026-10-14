package quota

import (
	"strings"
	"testing"
	"time"
)

const copilotSample = `{
  "plan": "individual",
  "quota_reset_date": "2026-11-01",
  "quota_snapshots": {
    "premium_interactions": {"remaining": 26.6, "limit": 200, "percent_remaining": 13.3, "unlimited": false},
    "chat": {"unlimited": true},
    "completions": {"unlimited": true}
  }
}`

func TestParseCopilot(t *testing.T) {
	r := ParseCopilot(copilotSample, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}
	if got := *r.Used; got < 86 || got > 87 {
		t.Fatalf("used=%v, want ~86.7", got)
	}
	if !strings.Contains(r.Detail, "Individual") || !strings.Contains(r.Detail, "premium") {
		t.Fatalf("detail=%q", r.Detail)
	}
}

func TestParseCopilotUnlimited(t *testing.T) {
	r := ParseCopilot(`{"plan":"individual","quota_snapshots":{"chat":{"unlimited":true}}}`, time.Now().UTC())
	if !r.OK() || *r.Used != 0 || !strings.Contains(r.Detail, "Unlimited") {
		t.Fatalf("got %+v", r)
	}
}

func TestParseCopilotBadJSON(t *testing.T) {
	if r := ParseCopilot(`{`, time.Now().UTC()); r.OK() {
		t.Fatalf("expected failure")
	}
}

func TestCopilotMissingGH(t *testing.T) {
	if r := CopilotWith(""); r.OK() {
		t.Fatalf("expected failure without gh")
	}
}
