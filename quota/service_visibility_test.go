package quota

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// The parent sends the hidden set with every visibility change, so an empty
// list is a real request to show every agent again.
func TestDecodeAgentsRequestCarriesTheHiddenSet(t *testing.T) {
	req, err := DecodeRequest([]byte(`{"v":1,"op":"agents","hidden":["cursor","nope"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Op != "agents" || len(req.Hidden) != 2 || req.Hidden[0] != "cursor" {
		t.Fatalf("agents = %+v", req)
	}
	shown, err := DecodeRequest([]byte(`{"v":1,"op":"agents","hidden":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(shown.Hidden) != 0 {
		t.Fatalf("empty hidden set = %+v", shown)
	}
}

func TestSnapshotOmitsHiddenAgents(t *testing.T) {
	vis := NewVisibility()
	vis.SetHidden([]string{ItemClaude, ItemCursor, "not-an-agent"})
	p := NewPoller(
		func(id string) Reading { return Fail("unused") },
		vis.On,
		func() {},
	)
	s := NewServiceWith(p, vis)

	snap := s.Snapshot()
	if len(snap.Readings) != len(ItemAll)-2 {
		t.Fatalf("readings = %d, want %d", len(snap.Readings), len(ItemAll)-2)
	}
	for _, r := range snap.Readings {
		if r.ID == ItemClaude || r.ID == ItemCursor {
			t.Fatalf("hidden agent %s was sent", r.ID)
		}
	}
}

// Hiding must stop the work, not just the painting: the poller's isOn is the
// shared Visibility, so Refresh starts only the agents still shown.
func TestHiddenAgentsAreNotRead(t *testing.T) {
	vis := NewVisibility()
	vis.SetHidden([]string{ItemClaude, ItemCursor})

	var mu sync.Mutex
	var read []string
	var wg sync.WaitGroup
	wg.Add(len(ItemAll) - 2)
	p := NewPoller(func(id string) Reading {
		mu.Lock()
		read = append(read, id)
		mu.Unlock()
		wg.Done()
		return newReading(1, "ok")
	}, vis.On, func() {})
	p.Refresh(true)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(read) != len(ItemAll)-2 {
		t.Fatalf("reads = %v, want %d", read, len(ItemAll)-2)
	}
	for _, id := range read {
		if id == ItemClaude || id == ItemCursor {
			t.Fatalf("hidden agent %s was read", id)
		}
	}
}

func TestAgentsRequestHidesAndResumes(t *testing.T) {
	vis := NewVisibility()
	p := NewPoller(
		func(id string) Reading { return Fail("unused") },
		vis.On,
		func() {},
	)
	s := NewServiceWith(p, vis)

	events, done := s.Handle(`{"v":1,"op":"agents","hidden":["claude"]}`)
	if done || len(events) != 1 || events[0].Op != "snapshot" {
		t.Fatalf("events = %+v, done = %v", events, done)
	}
	for _, r := range events[0].Readings {
		if r.ID == ItemClaude {
			t.Fatal("hidden claude was sent")
		}
	}
	if vis.On(ItemClaude) {
		t.Fatal("claude is still on")
	}

	resumed, _ := s.Handle(`{"v":1,"op":"agents","hidden":[]}`)
	if len(resumed) != 1 || len(resumed[0].Readings) != len(ItemAll) {
		t.Fatalf("resumed snapshot = %+v", resumed)
	}
	if !vis.On(ItemClaude) {
		t.Fatal("claude did not come back")
	}
}

// Bloom includes visibility in hello so no hidden read can start first.
func TestStartupVisibilityPrecedesFirstRead(t *testing.T) {
	visible := NewVisibility()
	reads := make(chan string, len(ItemAll))
	poller := NewPoller(func(id string) Reading { reads <- id; return newReading(1, "ok") }, visible.On, func() {})
	service := NewServiceWith(poller, visible)
	hidden := strings.Join(ItemAll[:], `","`)
	input := `{"v":1,"op":"hello","hidden":["` + hidden + `"]}` + "\n"
	if err := Serve(strings.NewReader(input), io.Discard, service); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-reads:
		t.Fatalf("hidden agent read at startup: %s", id)
	case <-time.After(20 * time.Millisecond):
	}
}
