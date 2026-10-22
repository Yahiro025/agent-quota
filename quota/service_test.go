package quota

import (
	"bufio"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func findAgent(t *testing.T, ev ServiceEvent, id string) ServiceAgent {
	t.Helper()
	for _, r := range ev.Readings {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no reading for %s in %+v", id, ev)
	return ServiceAgent{}
}

func TestHelloRepliesReadyThenWaitingSnapshot(t *testing.T) {
	p := NewPoller(
		func(id string) Reading { return Fail("unused") },
		func(id string) bool { return true },
		func() {},
	)
	s := NewService(p)
	events, done := s.Handle(`{"v":1,"op":"hello"}`)
	if done {
		t.Fatal("hello ended the session")
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want ready + snapshot", len(events))
	}
	if events[0].Op != "ready" || events[0].V != 1 {
		t.Fatalf("first event = %+v", events[0])
	}
	if !slices.Equal(events[0].Agents, ItemAll[:]) {
		t.Fatalf("agents = %v", events[0].Agents)
	}
	snap := events[1]
	if snap.Op != "snapshot" || len(snap.Readings) != len(ItemAll) {
		t.Fatalf("snapshot = %+v", snap)
	}
	for i, r := range snap.Readings {
		if r.ID != ItemAll[i] || r.State != "waiting" {
			t.Fatalf("reading %d = %+v", i, r)
		}
	}
}

func TestRefreshOneLeavesOtherAgentsWaiting(t *testing.T) {
	var mu sync.Mutex
	var ids []string
	changed := make(chan struct{}, 4)
	p := NewPoller(func(id string) Reading {
		mu.Lock()
		ids = append(ids, id)
		mu.Unlock()
		if id == "claude" {
			return newReading(12, "ok")
		}
		return Fail("unused")
	}, func(id string) bool { return true }, func() { changed <- struct{}{} })
	s := NewService(p)

	events, done := s.Handle(`{"v":1,"op":"refresh","id":"claude"}`)
	if done || len(events) != 0 {
		t.Fatalf("refresh: events = %v, done = %v", events, done)
	}
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("no change was said")
	}

	snap := s.Snapshot()
	claude := findAgent(t, snap, "claude")
	if claude.State != "ok" || claude.Used == nil || *claude.Used != 12 {
		t.Fatalf("claude = %+v", claude)
	}
	if kiro := findAgent(t, snap, "kiro"); kiro.State != "waiting" {
		t.Fatalf("kiro = %+v", kiro)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(ids, []string{"claude"}) {
		t.Fatalf("reads = %v", ids)
	}
}

func TestStaleKeepsLastGoodUsed(t *testing.T) {
	var mu sync.Mutex
	reads := 0
	changed := make(chan struct{}, 4)
	p := NewPoller(func(id string) Reading {
		mu.Lock()
		reads++
		n := reads
		mu.Unlock()
		if n == 1 {
			return newReading(40, "ok")
		}
		return Fail("timeout")
	}, func(id string) bool { return true }, func() { changed <- struct{}{} })
	s := NewService(p)

	wait := func() {
		select {
		case <-changed:
		case <-time.After(5 * time.Second):
			t.Fatal("no change was said")
		}
	}
	s.Handle(`{"v":1,"op":"refresh","id":"claude"}`)
	wait()
	s.Handle(`{"v":1,"op":"refresh","id":"claude"}`)
	wait()

	claude := findAgent(t, s.Snapshot(), "claude")
	if claude.State != "stale" || claude.Used == nil || *claude.Used != 40 || claude.Err != "timeout" {
		t.Fatalf("claude = %+v", claude)
	}
}

func TestShutdownSaysByeAndServeReturns(t *testing.T) {
	p := NewPoller(
		func(id string) Reading { return Fail("unused") },
		func(id string) bool { return true },
		func() {},
	)
	s := NewService(p)
	in := strings.NewReader("{\"v\":1,\"op\":\"nope\"}\n{\"v\":1,\"op\":\"shutdown\"}\n")
	var out strings.Builder
	if err := Serve(in, &out, s); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %v", lines)
	}
	var ev ServiceEvent
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Op != "error" || ev.Message == "" {
		t.Fatalf("first line = %s", lines[0])
	}
	if err := json.Unmarshal([]byte(lines[1]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Op != "bye" || ev.V != 1 {
		t.Fatalf("second line = %s", lines[1])
	}
}

func TestServeWritesSnapshotWhenThePollerChanges(t *testing.T) {
	var s *Service
	p := NewPoller(
		func(id string) Reading { return newReading(12, "ok") },
		func(id string) bool { return true },
		func() { s.OnChanged() },
	)
	s = NewService(p)

	reqR, reqW := io.Pipe()
	outR, outW := io.Pipe()
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(reqR, outW, s) }()

	lines := make(chan string, 32)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	if _, err := io.WriteString(reqW, "{\"v\":1,\"op\":\"hello\"}\n"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(10 * time.Second)
	for {
		select {
		case line := <-lines:
			var ev ServiceEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatal(err)
			}
			if ev.Op == "snapshot" && strings.Contains(line, `"used":12`) {
				goto changed
			}
		case <-deadline:
			t.Fatal("no snapshot carried the new reading")
		}
	}
changed:
	if _, err := io.WriteString(reqW, "{\"v\":1,\"op\":\"shutdown\"}\n"); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case line := <-lines:
			var ev ServiceEvent
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatal(err)
			}
			if ev.Op == "bye" {
				goto bye
			}
		case <-deadline:
			t.Fatal("no bye")
		}
	}
bye:
	if err := <-serveErr; err != nil {
		t.Fatal(err)
	}
	outW.Close()
}

func TestServeRefreshOfDisabledAgentDoesNotDeadlock(t *testing.T) {
	var s *Service
	var onMu sync.Mutex
	on := true
	p := NewPoller(
		func(id string) Reading { return newReading(12, "ok") },
		func(id string) bool {
			onMu.Lock()
			defer onMu.Unlock()
			return on
		},
		func() { s.OnChanged() },
	)
	s = NewService(p)

	reqR, reqW := io.Pipe()
	outR, outW := io.Pipe()
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(reqR, outW, s) }()

	lines := make(chan string, 32)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	if _, err := io.WriteString(reqW, "{\"v\":1,\"op\":\"refresh\",\"id\":\"claude\"}\n"); err != nil {
		t.Fatal(err)
	}
	// The changed snapshot only leaves after the read is stored, so claude
	// has a real reading by the time the agent is switched off.
	select {
	case <-lines:
	case <-time.After(5 * time.Second):
		t.Fatal("no snapshot carried claude's reading")
	}
	if st, _, _, ok := p.Display("claude"); !ok || st != OK {
		t.Fatal("claude's reading was not stored")
	}

	onMu.Lock()
	on = false
	onMu.Unlock()

	// Both lines go in one write: while Serve is deadlocked its scanner
	// never reads again, so a later write would block this goroutine too.
	if _, err := io.WriteString(reqW, "{\"v\":1,\"op\":\"refresh\",\"id\":\"claude\"}\n{\"v\":1,\"op\":\"shutdown\"}\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve deadlocked refreshing an agent that is off")
	}
	outW.Close()
}
