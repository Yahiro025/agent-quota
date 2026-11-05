package quota

import (
	"reflect"
	"testing"
	"time"
)

func TestQuotaDetailsPreserveSourceFacts(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	five, week := now.Add(time.Hour), now.Add(7*24*time.Hour)
	plan := "pro"
	claude := ParseClaudeUsage(`{"five_hour":{"utilization":19,"resets_at":"2026-10-16T01:00:00Z"},"seven_day":{"utilization":70,"resets_at":"2026-10-23T00:00:00Z"}}`, &plan, now)
	codex, ok := ParseCodexLine(`{"timestamp":"2026-10-16T00:00:00Z","payload":{"rate_limits":{"primary":{"used_percent":68,"resets_in_seconds":3600},"secondary":{"used_percent":37,"resets_in_seconds":604800}}}}`, now)
	if !ok {
		t.Fatal("Codex fixture rejected")
	}
	devin := ParseDevin(`{"dailyQuotaRemainingPercent":80,"weeklyQuotaRemainingPercent":62,"dailyQuotaResetAtUnix":"1792112400","weeklyQuotaResetAtUnix":"1792713600"}`, now)
	for _, tc := range []struct {
		name string
		got  Reading
		want []Detail
	}{
		{"claude", claude, []Detail{{Label: "5h", Value: "19% used", ResetAt: five}, {Label: "Week", Value: "70% used", ResetAt: week}}},
		{"codex", codex, []Detail{{Label: "5h", Value: "68% used", ResetAt: five}, {Label: "Week", Value: "37% used", ResetAt: week}}},
		{"devin", devin, []Detail{{Label: "Day", Value: "20% used", ResetAt: five}, {Label: "Week", Value: "38% used", ResetAt: week}}},
	} {
		if !reflect.DeepEqual(tc.got.Details, tc.want) {
			t.Errorf("%s details=%#v, want %#v", tc.name, tc.got.Details, tc.want)
		}
	}
	if !codex.ObservedAt.Equal(now) {
		t.Fatalf("Codex observation=%v", codex.ObservedAt)
	}
	untimed, ok := ParseCodexLine(`{"payload":{"rate_limits":{"primary":{"used_percent":1,"resets_in_seconds":3600}}}}`, now)
	if !ok || !untimed.ObservedAt.IsZero() || !untimed.Details[0].ResetAt.Equal(five) {
		t.Fatalf("missing source time=%#v", untimed)
	}
	kiro := ParseKiro("KIRO FREE\n(1.66 of 50 covered in plan) (resets on 11/01)")
	if want := []Detail{{Label: "Credits", Value: "1.66 of 50 credits", ResetText: "11/01"}}; !reflect.DeepEqual(kiro.Details, want) {
		t.Fatalf("Kiro=%#v", kiro.Details)
	}
}

func TestCursorDashboardBucketsReplaceAllowanceRows(t *testing.T) {
	r := ParseCursorSummary(`{"membershipType":"pro","billingCycleEnd":"2026-11-01T00:00:00Z","individualUsage":{"plan":{"totalPercentUsed":66,"autoPercentUsed":20,"apiPercentUsed":80,"used":1234,"limit":2000},"overall":{"used":1500,"limit":3000}},"teamUsage":{"pooled":{"used":4000,"limit":10000}}}`)
	reset := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	want := []Detail{{Label: "Cursor Models", Value: "20% used", ResetAt: reset}, {Label: "Other Models", Value: "80% used", ResetAt: reset}}
	if !r.OK() || *r.Used != 66 || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Cursor=%#v", r)
	}
}

func TestCopilotDetailsKeepFiniteAndUnlimitedCategories(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	r := ParseCopilot(`{"plan":"individual","quota_reset_date":"2026-11-01","quota_snapshots":{"premium_interactions":{"percent_remaining":87,"remaining":174,"limit":200},"chat":{"unlimited":true},"completions":{"percent_remaining":90}}}`, now)
	want := []Detail{{Label: "premium", Value: "13% used · 26 of 200 used · 174 remaining", ResetText: "2026-11-01"}, {Label: "chat", Value: "Unlimited", ResetText: "2026-11-01"}, {Label: "completions", Value: "10% used", ResetText: "2026-11-01"}}
	if !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Copilot=%#v", r.Details)
	}
	for _, raw := range []string{`{"quota_snapshots":{}}`, `{"quota_snapshots":{"chat":{}}}`} {
		if r := ParseCopilot(raw, now); r.OK() {
			t.Fatalf("missing usage became successful: %#v", r)
		}
	}
	u := ParseCopilot(`{"quota_reset_date":"2026-11-01","quota_snapshots":{"chat":{"unlimited":true}}}`, now)
	if !u.OK() || u.Summary != "Unlimited" || len(u.Details) != 1 || u.Details[0].ResetText != "2026-11-01" {
		t.Fatalf("all unlimited=%#v", u)
	}
}

func TestAntigravityDetailsKeepDuplicateWindowBuckets(t *testing.T) {
	r := ParseAntigravity(`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini","buckets":[{"id":"fast","name":"Fast","window":"5h","remaining_fraction":0.8,"reset_time":"2026-10-16T01:00:00Z"},{"id":"pro","window":"5h","remaining_fraction":0.4},{"id":"off","disabled":true,"window":"week","remaining_fraction":0}]}]}}}`, time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if !r.OK() || len(r.Details) != 2 {
		t.Fatalf("Antigravity=%#v", r)
	}
	if r.Details[0].Label != "Gemini / Fast / 5h" || r.Details[1].Label != "Gemini / pro / 5h" || r.Details[1].Value != "60% used" || r.Details[0].ResetAt.IsZero() {
		t.Fatalf("bucket facts=%#v", r.Details)
	}
}

func TestCopilotCountOnlyCategoryDoesNotInventPercent(t *testing.T) {
	r := ParseCopilot(`{"quota_snapshots":{"premium_interactions":{"percent_remaining":87},"chat":{"remaining":12,"limit":20}}}`, time.Now())
	want := []Detail{{Label: "premium", Value: "13% used"}, {Label: "chat", Value: "12 remaining · 20 limit"}}
	if !r.OK() || *r.Used != 13 || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("counts=%#v", r)
	}
	r = ParseCopilot(`{"quota_snapshots":{"chat":{"unlimited":true},"completions":{}}}`, time.Now())
	if r.OK() {
		t.Fatalf("unknown finite category became Unlimited: %#v", r)
	}
}

func TestDevinGeneralResetIsNotAssignedToAWindow(t *testing.T) {
	r := ParseDevin(`{"dailyQuotaRemainingPercent":80,"weeklyQuotaRemainingPercent":62,"resetAt":1792112400}`, time.Now())
	if len(r.Details) != 3 || !r.Details[0].ResetAt.IsZero() || !r.Details[1].ResetAt.IsZero() || r.Details[2].Label != "General reset" || r.Details[2].Value != FormatLocal(time.Unix(1792112400, 0), "d MMM HH:mm") {
		t.Fatalf("general reset=%#v", r.Details)
	}
}

func TestStaleRetainsDetailsAndSourceObservation(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	b := NewBook()
	r := newReading(42, "42%")
	r.Details = []Detail{{Label: "Week", Value: "42% used", ResetAt: now.Add(time.Hour)}}
	r.ObservedAt = now.Add(-time.Minute)
	b.Finished(ItemCodex, r, true, now)
	b.Finished(ItemCodex, Fail("connection failed"), true, now.Add(time.Minute))
	state, show, reason, _ := b.StateOf(ItemCodex)
	if state != Stale || reason != "connection failed" || !reflect.DeepEqual(show.Details, r.Details) || !show.ObservedAt.Equal(r.ObservedAt) {
		t.Fatalf("stale=%v %#v %s", state, show, reason)
	}
}

func TestClaudeSnapshotDetailsKeepBothResetsAndOriginalObservation(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	five, week := now.Add(time.Hour), now.Add(48*time.Hour)
	input := snapshotInput("details", 40, five.Unix(), week.Unix(), "1")
	if !CaptureClaudeSnapshot(input, now) {
		t.Fatal("snapshot not accepted")
	}
	CaptureClaudeSnapshot(input, now.Add(time.Minute))
	r, ok := claudeSnapshotReading(now.Add(time.Minute))
	want := []Detail{{Label: "5h", Value: "40% used", ResetAt: five}, {Label: "Week", Value: "20% used", ResetAt: week}}
	if !ok || !reflect.DeepEqual(r.Details, want) || !r.ObservedAt.Equal(now) {
		t.Fatalf("snapshot=%#v ok=%v", r, ok)
	}
}
