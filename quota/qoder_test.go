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

const sampleQoderUsage = `{"displayMode":"qoder","qoderUsage":{"userType":"personal_professional_trial","usageType":"credits","expiresAt":1794682223610,"isQuotaExceeded":false,"totalUsagePercentage":0.38,"userQuota":{"percentage":0.72,"total":300,"used":215,"remaining":85,"unit":"credits"},"addOnQuota":{"percentage":0.04,"total":300,"used":12,"remaining":288,"unit":"credits"}}}`

func TestQoderSuccessMapping(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer qoder-token" {
			t.Errorf("Authorization = %q, want Bearer qoder-token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		if got := r.Header.Get("Cosy-ClientType"); got != "10" {
			t.Errorf("Cosy-ClientType = %q, want 10", got)
		}
		if got := r.Header.Get("User-Agent"); got != "Qoder" {
			t.Errorf("User-Agent = %q, want Qoder", got)
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sampleQoderUsage)
	}))
	defer srv.Close()

	r := QoderWith(srv.URL, "qoder-token", now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}

	// Used headlines userQuota percentage (72%)
	if r.Used == nil || *r.Used != 72 {
		t.Fatalf("used = %v, want 72", r.Used)
	}

	if r.Plan != "Pro Trial" {
		t.Fatalf("plan = %q, want Pro Trial", r.Plan)
	}

	// Reset5h must NOT be set
	if !r.Reset5h.IsZero() {
		t.Fatalf("Reset5h = %v, want zero time", r.Reset5h)
	}

	if r.ResetLabel != "" {
		t.Fatalf("ResetLabel = %q, want empty", r.ResetLabel)
	}

	if len(r.Details) != 3 {
		t.Fatalf("details = %+v, want 3 details (plan, add-on, plan ends)", r.Details)
	}
	if r.Details[0].Label != "Plan credits" || r.Details[0].Value != "72% used · 215 / 300 credits" {
		t.Errorf("details[0] = %+v", r.Details[0])
	}
	if r.Details[1].Label != "Add-on credits" || r.Details[1].Value != "4% used · 12 / 300 credits" {
		t.Errorf("details[1] = %+v", r.Details[1])
	}
	if r.Details[2].Label != "Plan ends" || !r.Details[2].ResetAt.IsZero() || r.Details[2].ResetText != "" || r.Details[2].Value == "" {
		t.Errorf("details[2] = %+v", r.Details[2])
	}

	if !strings.Contains(r.Detail, "Pro Trial") || !strings.Contains(r.Detail, "72% of plan") || !strings.Contains(r.Detail, " · ends ") || strings.Contains(r.Detail, "resets") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestQoderQuotaExceeded(t *testing.T) {
	now := time.Now()
	raw := `{"displayMode":"qoder","qoderUsage":{"userType":"personal_pro","isQuotaExceeded":true,"userQuota":{"percentage":0.5,"total":100,"used":50}}}`
	r := ParseQoderUsage(raw, now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}
	if r.Used == nil || *r.Used != 100 {
		t.Fatalf("used = %v, want 100 when isQuotaExceeded is true", r.Used)
	}
}

func TestQoderEnterpriseMode(t *testing.T) {
	now := time.Now()
	raw := `{"displayMode":"enterprise","qoderUsage":{}}`
	r := ParseQoderUsage(raw, now)
	if r.OK() {
		t.Fatal("expected failure on enterprise mode")
	}
	want := "Qoder enterprise usage isn't shown here yet."
	if r.Detail != want {
		t.Fatalf("detail = %q, want %q", r.Detail, want)
	}
}

func TestQoderAuthRefused(t *testing.T) {
	now := time.Now()
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		r := QoderWith(srv.URL, "bad-token", now)
		srv.Close()
		if r.OK() {
			t.Fatalf("status %d returned OK reading", status)
		}
		wantMsg := "openapi.qoder.sh refused Qoder’s sign-in — open Qoder to renew it."
		if r.Detail != wantMsg {
			t.Fatalf("detail = %q, want %q", r.Detail, wantMsg)
		}
	}
}

func TestQoderRateLimitBackoff(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	r1 := QoderWith(srv.URL, "tok", t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	if r1.OK() || !strings.Contains(r1.Detail, "Qoder is limiting usage checks") {
		t.Fatalf("r1 detail = %q", r1.Detail)
	}

	// Second read inside backoff: 0 HTTP calls
	r2 := QoderWith(srv.URL, "tok", t0.Add(30*time.Second))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	if r2.OK() || !strings.Contains(r2.Detail, "Qoder is limiting usage checks") {
		t.Fatalf("r2 detail = %q", r2.Detail)
	}
}

func TestParseQoderAuthJSON(t *testing.T) {
	// Top-level token and ms expiresAt
	t1, exp1, err1 := parseQoderAuthJSON(`{"token":"top-token","expiresAt":1794682223610}`)
	if err1 != nil || t1 != "top-token" || exp1 == nil {
		t.Fatalf("top-level auth JSON failed: %v, %q, %v", err1, t1, exp1)
	}

	// Nested under auth with ISO string
	t2, exp2, err2 := parseQoderAuthJSON(`{"auth":{"token":"nested-token","expiresAt":"2026-11-15T12:00:00Z"}}`)
	if err2 != nil || t2 != "nested-token" || exp2 == nil {
		t.Fatalf("nested auth JSON failed: %v, %q, %v", err2, t2, exp2)
	}

	// Bad JSON
	if _, _, err := parseQoderAuthJSON("not json"); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestQoderExpiredTokenMakesNoHTTPCall(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sampleQoderUsage)
	}))
	defer srv.Close()

	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)

	r := QoderToken(srv.URL, "expired-token", &past, now)
	if r.OK() {
		t.Fatal("expected failure on expired token")
	}
	want := "Qoder's sign-in has expired — open Qoder to renew it."
	if r.Detail != want {
		t.Fatalf("detail = %q, want %q", r.Detail, want)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("expired token sent %d HTTP requests, want 0", got)
	}
}
