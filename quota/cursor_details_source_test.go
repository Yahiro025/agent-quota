package quota

import (
	"reflect"
	"testing"
	"time"
)

func TestCursorDetailsRetainUsageSummarySourceFacts(t *testing.T) {
	r := ParseCursorSummary(`{"membershipType":"pro","billingCycleEnd":"2026-10-28T13:03:10.000Z","individualUsage":{"plan":{"enabled":true,"totalPercentUsed":66.12682926829268,"autoPercentUsed":65.22666666666666,"apiPercentUsed":100,"used":2000,"limit":2000,"remaining":0,"breakdown":{"included":2000,"bonus":28501,"total":30501}},"onDemand":{"enabled":false,"used":0,"limit":null,"remaining":null}},"teamUsage":{}}`)
	reset := time.Date(2026, 10, 28, 13, 3, 10, 0, time.UTC)
	want := []Detail{
		{Label: "Cursor Models", Value: "65% used", ResetAt: reset},
		{Label: "Other Models", Value: "100% used", ResetAt: reset},
	}
	if !r.OK() || *r.Used != 66.12682926829268 || r.Plan != "Pro" || !reflect.DeepEqual(r.Details, want) {
		t.Fatalf("Cursor reading=%#v, want details=%#v", r, want)
	}
}

func TestCursorDetailsDistinguishZeroAndMissingSourceValues(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		want        []Detail
	}{
		{"zero", `"plan":{"totalPercentUsed":1,"used":0,"limit":0,"remaining":0},"onDemand":{"enabled":true,"used":0,"limit":0,"remaining":0}`, []Detail{{Label: "Plan", Value: "0 of 0 used · 0 remaining"}, {Label: "On-demand", Value: "On · 0 of 0 used · 0 remaining"}}},
		{"missing", `"plan":{"totalPercentUsed":1},"onDemand":{}`, nil},
		{"null", `"plan":{"totalPercentUsed":1,"used":null,"limit":null,"remaining":null},"onDemand":{"enabled":null,"used":null,"limit":null,"remaining":null}`, nil},
		{"remaining only", `"plan":{"totalPercentUsed":1,"remaining":5},"onDemand":{"enabled":false}`, []Detail{{Label: "Plan", Value: "5 remaining"}, {Label: "On-demand", Value: "Off"}}},
		{"limit only", `"plan":{"totalPercentUsed":1},"onDemand":{"limit":12}`, []Detail{{Label: "On-demand", Value: "12 limit"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ParseCursorSummary(`{"individualUsage":{` + tc.usage + `}}`)
			if !r.OK() || *r.Used != 1 || !reflect.DeepEqual(r.Details, tc.want) {
				t.Fatalf("reading=%#v, want details=%#v", r, tc.want)
			}
		})
	}
}

func TestCursorDetailsKeepOnlyReportedPlanBreakdownValues(t *testing.T) {
	for _, tc := range []struct {
		name, breakdown string
		want            []Detail
	}{
		{"included zero", `{"included":0}`, []Detail{{Label: "Plan breakdown", Value: "Included 0"}}},
		{"bonus only", `{"bonus":42}`, []Detail{{Label: "Plan breakdown", Value: "Bonus 42"}}},
		{"total only", `{"total":50}`, []Detail{{Label: "Plan breakdown", Value: "Total 50"}}},
		{"null", `{"included":null,"bonus":null,"total":null}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := ParseCursorSummary(`{"individualUsage":{"plan":{"totalPercentUsed":1,"breakdown":` + tc.breakdown + `}}}`)
			if !r.OK() || *r.Used != 1 || !reflect.DeepEqual(r.Details, tc.want) {
				t.Fatalf("reading=%#v, want details=%#v", r, tc.want)
			}
		})
	}
}
