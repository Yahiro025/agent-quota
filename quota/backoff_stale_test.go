package quota

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestBlocked429KeepsLastGoodReadingAndForcedRefreshMakesNoHTTPCall verifies that:
// 1. An initial successful read populates Book with a good reading (state OK).
// 2. A subsequent 429 response transitions Book to Stale, retaining the good reading and recording the 429 backoff error.
// 3. A forced refresh inside the rate-limit window returns the cached failure without making an HTTP call, keeping Book in Stale.
func TestBlocked429KeepsLastGoodReadingAndForcedRefreshMakesNoHTTPCall(t *testing.T) {
	ResetBackoff()
	t.Cleanup(ResetBackoff)

	var calls atomic.Int32
	var status atomic.Int32
	status.Store(http.StatusOK)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		code := int(status.Load())
		if code == http.StatusTooManyRequests {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"five_hour":{"utilization":40,"resets_at":null}}`))
	}))
	defer srv.Close()

	t0 := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	later := t0.Add(24 * time.Hour).UnixMilli()
	cred := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"tok","expiresAt":%d,"subscriptionType":"pro"}}`, later)

	// 1. First read succeeds against the server and records a good reading in Book.
	r1 := ClaudeWith(cred, srv.URL, t0)
	if !r1.OK() {
		t.Fatalf("expected r1 to be OK, got detail: %q", r1.Detail)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 server call, got %d", got)
	}

	b := NewBook()
	if !b.Finished(ItemClaude, r1, true, t0) {
		t.Fatal("expected r1 to be kept in Book")
	}
	st, show, err, ok := b.StateOf(ItemClaude)
	if !ok || st != OK {
		t.Fatalf("state=%v ok=%v, want OK", st, ok)
	}
	if show.Used == nil || *show.Used != 40 {
		t.Fatalf("show.Used=%v, want 40", show.Used)
	}
	if err != "" {
		t.Fatalf("err=%q, want empty", err)
	}

	// 2. Server transitions to 429 Too Many Requests; second read records the block.
	status.Store(http.StatusTooManyRequests)
	t1 := t0.Add(time.Minute)

	r2 := ClaudeWith(cred, srv.URL, t1)
	if r2.OK() {
		t.Fatal("expected r2 to fail with 429")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 server calls, got %d", got)
	}
	wantMsg := "Anthropic is limiting usage checks; trying again in five minutes."
	if r2.Detail != wantMsg {
		t.Fatalf("r2.Detail = %q, want %q", r2.Detail, wantMsg)
	}

	b.Finished(ItemClaude, r2, true, t1)
	st, show, err, ok = b.StateOf(ItemClaude)
	if !ok || st != Stale {
		t.Fatalf("state=%v ok=%v, want Stale", st, ok)
	}
	if show.Used == nil || *show.Used != 40 {
		t.Fatalf("show.Used=%v, want previous good reading 40", show.Used)
	}
	if err != wantMsg {
		t.Fatalf("err=%q, want %q", err, wantMsg)
	}

	// 3. Issue a forced refresh inside the 5-minute window; no HTTP request is made and Book remains Stale.
	t2 := t0.Add(2 * time.Minute)
	if !b.RefreshOne(ItemClaude, true, t2) {
		t.Fatal("forced RefreshOne failed to start")
	}
	r3 := ClaudeWith(cred, srv.URL, t2)
	if r3.OK() {
		t.Fatal("expected r3 to fail in backoff")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected server hit counter to remain 2, got %d", got)
	}
	b.Finished(ItemClaude, r3, true, t2)

	st, show, err, ok = b.StateOf(ItemClaude)
	if !ok || st != Stale {
		t.Fatalf("state=%v ok=%v, want Stale", st, ok)
	}
	if show.Used == nil || *show.Used != 40 {
		t.Fatalf("show.Used=%v, want previous good reading 40", show.Used)
	}
	wantRemainingMsg := "Anthropic is limiting usage checks; trying again in 4 minutes."
	if err != wantRemainingMsg {
		t.Fatalf("err=%q, want %q", err, wantRemainingMsg)
	}
}
