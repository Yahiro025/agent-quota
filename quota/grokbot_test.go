package quota

import (
	"testing"
)

func TestParseGrokBotUsage(t *testing.T) {
	reset, ok := ParseTime("2026-11-01T15:07:42.181Z")
	if !ok {
		t.Fatal("ParseTime fixture")
	}
	raw := `{"usagePercent":95.54793,"nextResetTimestampUtc":"2026-11-01T15:07:42.181Z","cursorPlanName":"Pro","grokPlanLabel":"Grok Bot Plan"}`
	r := ParseGrokBotUsage(raw)
	if !r.OK() || r.Used == nil || *r.Used != 95.54793 || r.Plan != "Pro" {
		t.Fatalf("reading=%#v", r)
	}
	if len(r.Details) != 1 || r.Details[0].Label != "Week" || r.Details[0].Value != "96% used" || !r.Details[0].ResetAt.Equal(reset) {
		t.Fatalf("details=%#v, want Week 96%% used reset=%v", r.Details, reset)
	}
	if ParseGrokBotUsage("{}").OK() {
		t.Fatal("{} should fail")
	}
	if ParseGrokBotUsage("[").OK() {
		t.Fatal("[ should fail")
	}
}
