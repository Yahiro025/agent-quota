package quota

import (
	"reflect"
	"testing"
	"time"
)

func TestInspectorWindowsKeepTheirSourcePercentages(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	plan := "pro"
	claude := ParseClaudeUsage(`{"five_hour":{"utilization":19},"seven_day":{"utilization":70}}`, &plan, now)
	codex, ok := ParseCodexLine(`{"payload":{"rate_limits":{"primary":{"used_percent":0},"secondary":{"used_percent":14}}}}`, now)
	if !ok {
		t.Fatal("Codex fixture was rejected")
	}
	devin := ParseDevin(`{"plan":"pro","dailyQuotaRemainingPercent":100,"weeklyQuotaRemainingPercent":62}`, now)
	for _, tc := range []struct {
		name    string
		r       Reading
		want    []Window
		maximum float64
	}{
		{"claude", claude, []Window{{Label: "5h", Used: 19}, {Label: "week", Used: 70}}, 70},
		{"codex", codex, []Window{{Label: "5h", Used: 0}, {Label: "week", Used: 14}}, 14},
		{"devin", devin, []Window{{Label: "day", Used: 0}, {Label: "week", Used: 38}}, 38},
	} {
		if !tc.r.OK() || *tc.r.Used != tc.maximum || !reflect.DeepEqual(tc.r.Windows, tc.want) {
			t.Errorf("%s reading = %#v, want max %v with windows %#v", tc.name, tc.r, tc.maximum, tc.want)
		}
	}
	if claude.Plan != "Pro" || devin.Plan != "Pro" {
		t.Fatal("source plan names were lost")
	}
	if !codex.Reset5h.IsZero() {
		t.Fatal("an unreported Codex reset was invented")
	}
}
