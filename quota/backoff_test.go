package quota

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitBackoffWithoutRetryAfter(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, t0.Add(24*time.Hour).UnixMilli())

	// 1. First read hits the server once and fails with new message.
	r1 := ClaudeWith(cred, srv.URL, t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "Anthropic is limiting usage checks; trying again in five minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// 2. Reads inside the 5-minute window make zero HTTP calls and return dynamic remaining time.
	r1m := ClaudeWith(cred, srv.URL, t0.Add(1*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	want1m := "Anthropic is limiting usage checks; trying again in 4 minutes."
	if r1m.OK() || r1m.Detail != want1m {
		t.Fatalf("r1m detail = %q, want %q", r1m.Detail, want1m)
	}

	r2 := ClaudeWith(cred, srv.URL, t0.Add(2*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call (no new HTTP requests), got %d", got)
	}
	want2m := "Anthropic is limiting usage checks; trying again in 3 minutes."
	if r2.OK() || r2.Detail != want2m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, want2m)
	}

	r4m30 := ClaudeWith(cred, srv.URL, t0.Add(4*time.Minute+30*time.Second))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	want4m30 := "Anthropic is limiting usage checks; trying again in a minute."
	if r4m30.OK() || r4m30.Detail != want4m30 {
		t.Fatalf("r4m30 detail = %q, want %q", r4m30.Detail, want4m30)
	}

	// 3. Read past the 5-minute window calls the server again.
	r3 := ClaudeWith(cred, srv.URL, t0.Add(6*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls after window expired, got %d", got)
	}
	if r3.OK() || r3.Detail != wantMsg {
		t.Fatalf("r3 detail = %q, want %q", r3.Detail, wantMsg)
	}
}

func TestRateLimitBackoffWithRetryAfterSeconds(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "600") // 10 minutes
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, t0.Add(24*time.Hour).UnixMilli())

	// First read: 1 call, 10 minutes message.
	r1 := ClaudeWith(cred, srv.URL, t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "Anthropic is limiting usage checks; trying again in 10 minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// Inside window at 8 min: 0 HTTP calls, remaining is 2 minutes.
	r2 := ClaudeWith(cred, srv.URL, t0.Add(8*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call (0 new HTTP calls), got %d", got)
	}
	wantMsg8m := "Anthropic is limiting usage checks; trying again in 2 minutes."
	if r2.OK() || r2.Detail != wantMsg8m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, wantMsg8m)
	}

	// Past window at 11 min: calls server again.
	r3 := ClaudeWith(cred, srv.URL, t0.Add(11*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	if r3.OK() || r3.Detail != wantMsg {
		t.Fatalf("r3 detail = %q, want %q", r3.Detail, wantMsg)
	}
}

func TestRateLimitBackoffWithRetryAfterHTTPDate(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	retryDate := t0.Add(15 * time.Minute)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", retryDate.Format(http.TimeFormat))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, t0.Add(24*time.Hour).UnixMilli())

	r1 := ClaudeWith(cred, srv.URL, t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "Anthropic is limiting usage checks; trying again in 15 minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// Inside window at 10 min: zero HTTP calls, remaining is 5 minutes.
	r2 := ClaudeWith(cred, srv.URL, t0.Add(10*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg5m := "Anthropic is limiting usage checks; trying again in five minutes."
	if r2.OK() || r2.Detail != wantMsg5m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, wantMsg5m)
	}

	// Past window at 16 min: calls server again.
	r3 := ClaudeWith(cred, srv.URL, t0.Add(16*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	if r3.OK() {
		t.Fatal("expected r3 to fail")
	}
}

func TestRateLimitBackoffCappedAtOneHour(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "7200") // 2 hours
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, t0.Add(24*time.Hour).UnixMilli())

	// First read: capped at 1 hour = 60 minutes.
	r1 := ClaudeWith(cred, srv.URL, t0)
	wantMsg := "Anthropic is limiting usage checks; trying again in 60 minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// Inside 1-hour window (59 min): 0 HTTP calls, remaining is 1 minute.
	r2 := ClaudeWith(cred, srv.URL, t0.Add(59*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg1m := "Anthropic is limiting usage checks; trying again in a minute."
	if r2.OK() || r2.Detail != wantMsg1m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, wantMsg1m)
	}

	// Past 1-hour window (61 min): calls server again.
	r3 := ClaudeWith(cred, srv.URL, t0.Add(61*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
	if r3.OK() || r3.Detail != wantMsg {
		t.Fatalf("r3 detail = %q, want %q", r3.Detail, wantMsg)
	}
}

func TestRateLimitBackoffWithRetryAfterTwentyMinutes(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1200") // 20 minutes
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, t0.Add(24*time.Hour).UnixMilli())

	// Read with Retry-After of 20 minutes says "20 minutes" at start.
	r1 := ClaudeWith(cred, srv.URL, t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "Anthropic is limiting usage checks; trying again in 20 minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}
}

func TestDevinRateLimitBackoff(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	r1 := DevinWith(srv.URL, "k", "1.2.3", t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "Devin is limiting usage checks; trying again in five minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// Second read inside window: 0 HTTP calls, remaining is 3 minutes.
	r2 := DevinWith(srv.URL, "k", "1.2.3", t0.Add(2*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg3m := "Devin is limiting usage checks; trying again in 3 minutes."
	if r2.OK() || r2.Detail != wantMsg3m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, wantMsg3m)
	}

	// Past window: calls server again.
	_ = DevinWith(srv.URL, "k", "1.2.3", t0.Add(6*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestGrokBotRateLimitBackoff(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	r1 := GrokBotWith(srv.URL, "tok", t0)
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg := "api2.cursor.sh is limiting usage checks; trying again in five minutes."
	if r1.OK() || r1.Detail != wantMsg {
		t.Fatalf("r1 detail = %q, want %q", r1.Detail, wantMsg)
	}

	// Second read inside window: 0 HTTP calls, remaining is 3 minutes.
	r2 := GrokBotWith(srv.URL, "tok", t0.Add(2*time.Minute))
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
	wantMsg3m := "api2.cursor.sh is limiting usage checks; trying again in 3 minutes."
	if r2.OK() || r2.Detail != wantMsg3m {
		t.Fatalf("r2 detail = %q, want %q", r2.Detail, wantMsg3m)
	}

	// Past window: calls server again.
	_ = GrokBotWith(srv.URL, "tok", t0.Add(6*time.Minute))
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestBookRefreshOneClaudeIntervalIsFiveMinutes(t *testing.T) {
	b := NewBook()
	now := time.Unix(1000, 0)

	// First read starts immediately.
	if !b.RefreshOne(ItemClaude, false, now) {
		t.Fatal("expected first read of Claude to start")
	}
	// Reading completes successfully.
	b.Finished(ItemClaude, newReading(30, "ok"), true, now.Add(2*time.Second))

	// Next read at 30 seconds must NOT start (unlike 30s providers).
	if b.RefreshOne(ItemClaude, false, now.Add(30*time.Second)) {
		t.Fatal("Claude started at 30s, expected 5-minute interval")
	}

	// Next read at 4 minutes must still NOT start.
	if b.RefreshOne(ItemClaude, false, now.Add(4*time.Minute)) {
		t.Fatal("Claude started at 4 minutes, expected 5-minute interval")
	}

	// Next read at 5 minutes DOES start.
	if !b.RefreshOne(ItemClaude, false, now.Add(5*time.Minute)) {
		t.Fatal("Claude did not start at 5 minutes")
	}

	// Forced refresh starts immediately regardless of interval.
	b.Finished(ItemClaude, newReading(30, "ok"), true, now.Add(5*time.Minute+2*time.Second))
	if !b.RefreshOne(ItemClaude, true, now.Add(5*time.Minute+10*time.Second)) {
		t.Fatal("forced RefreshOne of Claude failed to start")
	}
}

func TestParseRetryAfterClampsHugeValues(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		header string
		want   time.Duration
		ok     bool
	}{
		{"60", 60 * time.Second, true},
		{"3600", 3600 * time.Second, true},
		{"7200", 3600 * time.Second, true},
		{"1000000000", 3600 * time.Second, true},
		{"999999999999999999", 3600 * time.Second, true},
		{"999999999999999999999999999999", 3600 * time.Second, true},
	}
	for _, c := range cases {
		d, ok := parseRetryAfter(c.header, now)
		if ok != c.ok || d != c.want {
			t.Errorf("parseRetryAfter(%q) = (%v, %v), want (%v, %v)", c.header, d, ok, c.want, c.ok)
		}
	}
}
