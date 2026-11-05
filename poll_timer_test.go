package main

import (
	"sync/atomic"
	"testing"
	"time"

	"agent-quota/quota"
)

func TestTimerHonorsHeavyReadersAndManualRefreshDoesNotOverlap(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	var reads atomic.Int32
	started, finished := make(chan int32, 4), make(chan struct{}, 4)
	release := make(chan struct{})
	defer close(release)
	a := &app{settings: settings{Visible: map[string]bool{"kiro": true}}, scale: 1}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		n := reads.Add(1)
		started <- n
		if n > 1 {
			<-release
		}
		used := 20.0
		return quota.Reading{Used: &used}
	}, a.isOn, func() { finished <- struct{}{} })
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("reader did not complete")
		}
	}
	a.poller.Refresh(false)
	wait(finished)
	<-started
	a.OnMenu(101)
	if a.poller.Busy("kiro") || reads.Load() != 1 {
		t.Fatal("timer bypassed the heavier CLI reader's five-minute cadence")
	}
	a.OnMenu(menuRefresh)
	select {
	case n := <-started:
		if n != 2 {
			t.Fatalf("timer started read %d, want 2", n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual refresh did not fetch a fresh reading")
	}
	a.OnMenu(101)
	if reads.Load() != 2 {
		t.Fatal("timer overlapped an in-flight reader")
	}
}
