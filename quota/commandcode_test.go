package quota

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sampleCommandCodeCredits = `{"credits":{"monthlyCredits":8.39,"purchasedCredits":0,"freeCredits":0},"windowLimits":{"limited":true,"fiveHour":{"used":0.4775,"cap":3,"exceeded":false,"resetAt":1793678066798},"weekly":{"used":1.6081,"cap":6,"exceeded":false,"resetAt":1794246841606}}}`
const sampleCommandCodeSubscriptions = `{"success":true,"data":{"status":"active","planId":"individual-go-v1","currentPeriodEnd":"2026-12-02T17:48:57.000Z"}}`

func TestCommandCodeSuccessMapping(t *testing.T) {
	now := time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC)
	var creditsCalls, subsCalls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := r.Header.Get("User-Agent"); got != "agent-quota" {
			t.Errorf("User-Agent = %q, want agent-quota", got)
		}

		switch r.URL.Path {
		case "/alpha/billing/credits":
			creditsCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sampleCommandCodeCredits)
		case "/alpha/billing/subscriptions":
			subsCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sampleCommandCodeSubscriptions)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	r := CommandCodeWith("test-key", srv.URL, now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}

	// 5h: 0.4775 / 3 * 100 = 15.9166...%
	// week: 1.6081 / 6 * 100 = 26.8016...%
	// Ring Used = max(5h, week)
	wantUsed := 1.6081 / 6.0 * 100.0
	if r.Used == nil || *r.Used < wantUsed-1e-6 || *r.Used > wantUsed+1e-6 {
		t.Fatalf("used = %v, want %v", *r.Used, wantUsed)
	}

	if r.Plan != "Go" {
		t.Fatalf("plan = %q, want Go", r.Plan)
	}

	wantReset5h, _ := fromUnixMs(1793678066798)
	if !r.Reset5h.Equal(wantReset5h) {
		t.Fatalf("Reset5h = %v, want %v", r.Reset5h, wantReset5h)
	}

	if len(r.Windows) != 2 {
		t.Fatalf("windows = %+v, want 2 windows", r.Windows)
	}
	if r.Windows[0].Label != "5h" || Custom(r.Windows[0].Used, 0) != "16" {
		t.Errorf("window[0] = %+v, want 5h 16%%", r.Windows[0])
	}
	if r.Windows[1].Label != "week" || Custom(r.Windows[1].Used, 0) != "27" {
		t.Errorf("window[1] = %+v, want week 27%%", r.Windows[1])
	}

	// Details: 5h, Week, Monthly credits
	if len(r.Details) != 3 {
		t.Fatalf("details = %+v, want 3 details", r.Details)
	}
	if r.Details[0].Label != "5h" || r.Details[0].Value != "16% used" || !r.Details[0].ResetAt.Equal(wantReset5h) {
		t.Errorf("details[0] = %+v", r.Details[0])
	}
	wantResetWeek, _ := fromUnixMs(1794246841606)
	if r.Details[1].Label != "Week" || r.Details[1].Value != "27% used" || !r.Details[1].ResetAt.Equal(wantResetWeek) {
		t.Errorf("details[1] = %+v", r.Details[1])
	}
	wantPeriodEnd, _ := ParseTime("2026-12-02T17:48:57.000Z")
	if r.Details[2].Label != "Monthly credits" || r.Details[2].Value != "8.39 remaining" || !r.Details[2].ResetAt.Equal(wantPeriodEnd) {
		t.Errorf("details[2] = %+v", r.Details[2])
	}

	if !strings.Contains(r.Detail, "Go") || !strings.Contains(r.Detail, "5h 16%") || !strings.Contains(r.Detail, "week 27%") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestCommandCodeExpiredWindow(t *testing.T) {
	// 10:00 UTC is after 5h resetAt (~03:54 UTC), but before weekly reset (~Nov 9)
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	r := ParseCommandCode(sampleCommandCodeCredits, "individual-go-v1", "2026-12-02T17:48:57.000Z", now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}

	// Ring falls back to the weekly %
	wantUsed := 1.6081 / 6.0 * 100.0
	if r.Used == nil || *r.Used < wantUsed-1e-6 || *r.Used > wantUsed+1e-6 {
		t.Fatalf("used = %v, want %v", *r.Used, wantUsed)
	}

	// No Reset5h when expired
	if !r.Reset5h.IsZero() {
		t.Fatalf("Reset5h = %v, want zero time for expired 5h window", r.Reset5h)
	}

	if len(r.Windows) != 2 {
		t.Fatalf("windows = %+v, want 2 windows", r.Windows)
	}
	if r.Windows[0].Label != "5h" || r.Windows[0].Used != 0 {
		t.Errorf("window[0] = %+v, want 5h 0%%", r.Windows[0])
	}
	if r.Windows[1].Label != "week" || Custom(r.Windows[1].Used, 0) != "27" {
		t.Errorf("window[1] = %+v, want week 27%%", r.Windows[1])
	}

	// 5h row shows 0% used, no ResetAt
	if len(r.Details) != 3 {
		t.Fatalf("details = %+v, want 3 details", r.Details)
	}
	if r.Details[0].Label != "5h" || r.Details[0].Value != "0% used" || !r.Details[0].ResetAt.IsZero() {
		t.Errorf("details[0] = %+v", r.Details[0])
	}
	wantResetWeek, _ := fromUnixMs(1794246841606)
	if r.Details[1].Label != "Week" || r.Details[1].Value != "27% used" || !r.Details[1].ResetAt.Equal(wantResetWeek) {
		t.Errorf("details[1] = %+v", r.Details[1])
	}

	if !strings.Contains(r.Detail, "5h 0%") || !strings.Contains(r.Detail, "week 27%") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestCommandCodePlanParsing(t *testing.T) {
	cases := []struct {
		planId string
		want   string
	}{
		{"individual-go-v1", "Go"},
		{"teams-pro", "Teams Pro"},
		{"teams-enterprise-v2", "Teams Enterprise"},
		{"starter", "Starter"},
		{"", ""},
	}
	for _, c := range cases {
		got := parseCommandCodePlan(c.planId)
		if got != c.want {
			t.Errorf("parseCommandCodePlan(%q) = %q, want %q", c.planId, got, c.want)
		}
	}
}

func TestCommandCodeSubscriptionsFailureKeepsReading(t *testing.T) {
	now := time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/alpha/billing/credits" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sampleCommandCodeCredits)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r := CommandCodeWith("test-key", srv.URL, now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}
	if r.Plan != "" {
		t.Errorf("plan = %q, want empty when subscriptions endpoint fails", r.Plan)
	}
	// Usage reading still populated
	if r.Used == nil {
		t.Fatal("used is nil")
	}
}

func TestCommandCodeSubscriptionsRateLimitBackoff(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/alpha/billing/credits" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sampleCommandCodeCredits)
		} else {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC)
	r1 := CommandCodeWith("key", srv.URL, t0)
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	// Initial reading is still returned OK without plan
	if !r1.OK() {
		t.Fatalf("expected OK reading, got %q", r1.Detail)
	}
	if r1.Plan != "" {
		t.Errorf("plan = %q, want empty", r1.Plan)
	}

	// Subsequent read is blocked by backoff on that host
	r2 := CommandCodeWith("key", srv.URL, t0.Add(time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls (no new HTTP requests), got %d", got)
	}
	if r2.OK() || !strings.Contains(r2.Detail, "Command Code is limiting usage checks") {
		t.Fatalf("r2 detail = %q", r2.Detail)
	}
}

func TestCommandCodeAuthRefused(t *testing.T) {
	now := time.Now()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		r := CommandCodeWith("bad-key", srv.URL, now)
		srv.Close()
		if r.OK() {
			t.Fatalf("status %d returned OK reading", status)
		}
		wantMsg := "Command Code refused your sign-in — run commandcode to renew it."
		if r.Detail != wantMsg {
			t.Fatalf("detail = %q, want %q", r.Detail, wantMsg)
		}
	}
}

func TestCommandCodeRateLimitBackoff(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	r1 := CommandCodeWith("key", srv.URL, t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	if r1.OK() || !strings.Contains(r1.Detail, "Command Code is limiting usage checks") {
		t.Fatalf("r1 detail = %q", r1.Detail)
	}

	// Second read inside backoff: 0 HTTP calls
	r2 := CommandCodeWith("key", srv.URL, t0.Add(time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	if r2.OK() || !strings.Contains(r2.Detail, "Command Code is limiting usage checks") {
		t.Fatalf("r2 detail = %q", r2.Detail)
	}
}

func TestCommandCodeBadJSONAndNoLimits(t *testing.T) {
	now := time.Now()
	// Bad JSON
	if r := ParseCommandCode("not json", "", "", now); r.OK() {
		t.Fatal("expected failure on bad JSON")
	}

	// limited is false -> unlimited precedent (OK reading, 0%, Summary/plan says "Unlimited")
	rawNotLimited := `{"credits":{"monthlyCredits":10},"windowLimits":{"limited":false}}`
	rNoPlan := ParseCommandCode(rawNotLimited, "", "", now)
	if !rNoPlan.OK() {
		t.Fatalf("not limited without plan failed: %q", rNoPlan.Detail)
	}
	if rNoPlan.Used == nil || *rNoPlan.Used != 0 {
		t.Errorf("used = %v, want 0", rNoPlan.Used)
	}
	if rNoPlan.Plan != "Unlimited" || rNoPlan.Summary != "Unlimited" || rNoPlan.Detail != "Unlimited" {
		t.Errorf("plan=%q summary=%q detail=%q", rNoPlan.Plan, rNoPlan.Summary, rNoPlan.Detail)
	}
	if len(rNoPlan.Details) != 1 || rNoPlan.Details[0].Label != "Monthly credits" || rNoPlan.Details[0].Value != "10 remaining" {
		t.Errorf("details = %+v", rNoPlan.Details)
	}

	rWithPlan := ParseCommandCode(rawNotLimited, "individual-go-v1", "", now)
	if !rWithPlan.OK() {
		t.Fatalf("not limited with plan failed: %q", rWithPlan.Detail)
	}
	if rWithPlan.Used == nil || *rWithPlan.Used != 0 {
		t.Errorf("used = %v, want 0", rWithPlan.Used)
	}
	if rWithPlan.Plan != "Go" || rWithPlan.Summary != "Unlimited" || rWithPlan.Detail != "Go · Unlimited" {
		t.Errorf("plan=%q summary=%q detail=%q", rWithPlan.Plan, rWithPlan.Summary, rWithPlan.Detail)
	}

	// missing windows
	rawNoWindows := `{"credits":{"monthlyCredits":10},"windowLimits":{"limited":true}}`
	if r := ParseCommandCode(rawNoWindows, "", "", now); r.OK() || r.Detail != "Command Code reported no usage limits." {
		t.Fatalf("no windows r = %+v", r)
	}
}
