package quota

import (
	"testing"
	"time"
)

func TestBookStale(t *testing.T) {
	b := NewBook()
	now := time.Now()
	good := newReading(42, "Pro · 5h 42%")
	if !b.Finished("claude", good, true, now) {
		t.Fatal("not kept")
	}
	if st, _, _, _ := b.StateOf("claude"); st != OK {
		t.Fatalf("state=%v, want OK", st)
	}
	bad := Fail("Couldn't reach api.anthropic.com.")
	b.Finished("claude", bad, true, now.Add(time.Minute))
	st, show, err, ok := b.StateOf("claude")
	if !ok || st != Stale {
		t.Fatalf("state=%v ok=%v, want Stale", st, ok)
	}
	if show.Used == nil || *show.Used != 42 {
		t.Fatalf("stale shows %+v, want last good 42", show)
	}
	if err == "" {
		t.Fatal("stale hides the error")
	}
	if !b.Stale("claude") {
		t.Fatal("Stale() false")
	}
	if _, _, ok := b.LastGood("claude"); !ok {
		t.Fatal("no last good")
	}
}

func TestBookFailed(t *testing.T) {
	b := NewBook()
	now := time.Now()
	b.Finished("cursor", Fail("nope"), true, now)
	if st, _, _, _ := b.StateOf("cursor"); st != Failed {
		t.Fatalf("state=%v, want Failed", st)
	}
}
