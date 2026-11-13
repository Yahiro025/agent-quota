package quota

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestCopilotDetailsMatchReportedSchema(t *testing.T) {
	now := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	r := ParseCopilot(`{"copilot_plan":"individual","quota_reset_date":"2026-11-01","quota_reset_date_utc":"2026-11-01T00:00:00.000Z","quota_snapshots":{"chat":{"unlimited":true,"entitlement":0,"remaining":0,"overage_count":0,"overage_entitlement":0,"overage_permitted":false},"completions":{"unlimited":true},"premium_interactions":{"unlimited":false,"percent_remaining":86.7,"remaining":173,"quota_remaining":173.4,"entitlement":200,"credits_used":26,"quota_reset_at":0,"timestamp_utc":"2026-10-16T09:39:34.347Z","overage_count":0,"overage_entitlement":0,"overage_permitted":false,"has_quota":true,"token_based_billing":true}}}`, now)
	reset := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	want := []Detail{
		{Label: "chat", Value: "Unlimited", ResetAt: reset},
		{Label: "completions", Value: "Unlimited", ResetAt: reset},
		{Label: "premium", Value: "13% used · 26 used · 200 entitlement · 173.4 remaining", ResetAt: reset},
		{Label: "premium overage", Value: "Off", ResetAt: reset},
	}
	if !r.OK() || math.Abs(*r.Used-13.3) > 0.00001 || r.Plan != "Individual" || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Copilot=%#v, want details %#v", r, want)
	}
	observed := time.Date(2026, 10, 16, 9, 39, 34, 347000000, time.UTC)
	if !r.ObservedAt.Equal(observed) {
		t.Fatalf("observation=%v, want %v", r.ObservedAt, observed)
	}
}

func TestCopilotSourceResetFallbacks(t *testing.T) {
	now := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, root, snapshot string
		want                 Detail
	}{
		{"snapshot wins", `"quota_reset_date_utc":"2026-11-01T00:00:00Z",`, `"quota_reset_at":1793494800,`, Detail{Label: "premium", Value: "13% used", ResetAt: time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)}},
		{"zero falls back", `"quota_reset_date_utc":"2026-11-01T00:00:00Z",`, `"quota_reset_at":0,`, Detail{Label: "premium", Value: "13% used", ResetAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}},
		{"past falls back", `"quota_reset_date_utc":"2026-11-01T00:00:00Z",`, `"quota_reset_at":1,`, Detail{Label: "premium", Value: "13% used", ResetAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}},
		{"invalid UTC keeps date", `"quota_reset_date_utc":"bad","quota_reset_date":"2026-11-01",`, ``, Detail{Label: "premium", Value: "13% used", ResetText: "2026-11-01"}},
		{"missing stays missing", ``, ``, Detail{Label: "premium", Value: "13% used"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ParseCopilot(`{`+tc.root+`"quota_snapshots":{"premium_interactions":{`+tc.snapshot+`"percent_remaining":87}}}`, now)
			if !r.OK() || !reflect.DeepEqual(r.Details, []Detail{tc.want}) || !r.ObservedAt.IsZero() {
				t.Fatalf("Copilot=%#v, want %#v", r, tc.want)
			}
		})
	}
}

func TestCopilotSourceCountsAndOverageRemainIndependent(t *testing.T) {
	r := ParseCopilot(`{"quota_snapshots":{"premium_interactions":{"percent_remaining":80,"quota_remaining":12.345,"remaining":12,"credits_used":9.876,"entitlement":20,"overage_count":2.5,"overage_entitlement":10,"overage_permitted":true},"chat":{"quota_remaining":0,"entitlement":0},"completions":{"unlimited":true,"entitlement":0}}}`, time.Now())
	want := []Detail{
		{Label: "premium", Value: "20% used · 9.876 used · 20 entitlement · 12.345 remaining"},
		{Label: "premium overage", Value: "On · 2.5 used · 10 entitlement"},
		{Label: "chat", Value: "0 remaining"},
		{Label: "completions", Value: "Unlimited"},
	}
	if !r.OK() || *r.Used != 20 || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Copilot=%#v, want %#v", r, want)
	}
}

func TestCopilotUnknownSourceUsageDoesNotBecomeUnlimited(t *testing.T) {
	for _, raw := range []string{
		`{"quota_snapshots":{"premium_interactions":{"entitlement":200,"quota_remaining":173.4,"credits_used":26}}}`,
		`{"quota_snapshots":{"chat":{"unlimited":true},"premium_interactions":{"unlimited":"false","percent_remaining":"86.7"}}}`,
		`{"quota_snapshots":{"premium_interactions":{"unlimited":false,"entitlement":0}}}`,
		`{"quota_snapshots":{"premium_interactions":null}}`,
	} {
		r := ParseCopilot(raw, time.Now())
		if r.OK() {
			t.Fatalf("unknown usage became successful: %#v", r)
		}
	}
}

func TestCopilotMalformedSourceAliasesKeepLegacyFacts(t *testing.T) {
	r := ParseCopilot(`{"copilot_plan":null,"plan":"individual","quota_snapshots":{"premium_interactions":{"percent_remaining":87,"quota_remaining":"bad","remaining":174,"entitlement":"bad","limit":200,"timestamp_utc":"bad"}}}`, time.Now())
	want := []Detail{{Label: "premium", Value: "13% used · 26 of 200 used · 174 remaining"}}
	if !r.OK() || r.Plan != "Individual" || *r.Used != 13 || !reflect.DeepEqual(r.Details, want) || !r.ObservedAt.IsZero() {
		t.Fatalf("Copilot=%#v, want %#v", r, want)
	}
}

func TestCopilotEachCategoryKeepsItsResetAndOverage(t *testing.T) {
	now := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	r := ParseCopilot(`{"quota_reset_date_utc":"2026-11-01T00:00:00Z","quota_snapshots":{"premium_interactions":{"percent_remaining":86.7,"quota_reset_at":1793494800},"chat":{"percent_remaining":40,"quota_reset_at":1793498400,"overage_count":3},"completions":{"unlimited":true,"overage_permitted":true}}}`, now)
	want := []Detail{
		{Label: "premium", Value: "13% used", ResetAt: time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)},
		{Label: "chat", Value: "60% used", ResetAt: time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)},
		{Label: "chat overage", Value: "3 used", ResetAt: time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC)},
		{Label: "completions", Value: "Unlimited", ResetAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{Label: "completions overage", Value: "On", ResetAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
	}
	if !r.OK() || *r.Used != 60 || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Copilot=%#v, want %#v", r, want)
	}
}
