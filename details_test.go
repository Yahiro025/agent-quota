package main

import (
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-quota/quota"
	"agent-quota/widget"
)

func TestDetailResetLabelUsesTheSharedClockAndPreservesSourceDates(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	reset := now.Add(72*time.Hour + 12*time.Minute)
	row := quota.Detail{ResetAt: reset}
	if got := detailResetLabel(row, now, false); got != "resets in 3d 0h 12m" {
		t.Fatalf("countdown=%q", got)
	}
	if got, want := detailResetLabel(row, now, true), "resets "+quota.FormatLocal(reset, "d MMM HH:mm"); got != want {
		t.Fatalf("clock=%q want %q", got, want)
	}
	if got := detailResetLabel(quota.Detail{ResetText: "11/01"}, now, false); got != "resets 11/01" {
		t.Fatalf("source date=%q", got)
	}
	if got := detailResetLabel(quota.Detail{}, now, false); got != "" {
		t.Fatalf("missing reset=%q", got)
	}
	five := now.Add(33 * time.Minute)
	panel, _ := resetLabels(five, now, false)
	if got := detailResetLabel(quota.Detail{ResetAt: five}, now, false); got != panel[len("5h "):] {
		t.Fatalf("five-hour detail=%q panel=%q", got, panel)
	}
}

func TestBuildViewPropagatesCompleteDetailsAndStaleObservation(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	reset := time.Now().Truncate(time.Minute).Add(3 * time.Hour)
	observed := reset.Add(-4 * time.Hour)
	used := 70.0
	source := quota.Reading{Used: &used, Plan: "Pro", Reset5h: reset, ObservedAt: observed, Details: []quota.Detail{{Label: "5h", Value: "19% used", ResetAt: reset}, {Label: "Week", Value: "70% used", ResetText: "2026-10-23"}}}
	var attempts atomic.Int32
	changed := make(chan struct{}, 2)
	a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}}, resetClock: map[string]bool{}}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		if attempts.Add(1) > 1 {
			return quota.Fail("connection failed")
		}
		return source
	}, func(id string) bool { return id == quota.ItemCodex }, func() { changed <- struct{}{} })
	refresh := func() {
		t.Helper()
		a.poller.Refresh(true)
		select {
		case <-changed:
		case <-time.After(2 * time.Second):
			t.Fatal("mock quota read did not finish")
		}
	}
	refresh()
	it := a.buildView().Items[0]
	if it.State != quota.OK || it.Plan != "Pro" || it.Observed != quota.FormatLocal(observed, "d MMM HH:mm") || len(it.Details) != 2 {
		t.Fatalf("OK seam=%#v", it)
	}
	if it.Details[0].Value != "19% used" || it.Details[0].Reset != strings.TrimPrefix(it.Reset, "5h ") || it.Details[1] != (widget.Detail{Label: "Week", Value: "70% used", Reset: "resets 2026-10-23"}) {
		t.Fatalf("quota facts=%#v reset=%q", it.Details, it.Reset)
	}
	if !strings.Contains(it.Details[0].Reset, strings.ReplaceAll(it.PillReset, "h", "h ")) {
		t.Fatalf("countdown detail=%q pill=%q", it.Details[0].Reset, it.PillReset)
	}
	a.resetClock[quota.ItemCodex] = true
	clock := a.buildView().Items[0]
	if clock.Details[0].Reset != strings.TrimPrefix(clock.Reset, "5h ") || clock.PillReset != quota.FormatLocal(reset, "HH:mm") || !strings.HasSuffix(clock.Details[0].Reset, clock.PillReset) {
		t.Fatalf("clock detail=%q row=%q pill=%q", clock.Details[0].Reset, clock.Reset, clock.PillReset)
	}
	refresh()
	stale := a.buildView().Items[0]
	if stale.State != quota.Stale || stale.Err != "connection failed" || stale.Plan != "Pro" || stale.Observed != it.Observed || !reflect.DeepEqual(stale.Details, clock.Details) || stale.Stale == "" {
		t.Fatalf("stale seam=%#v", stale)
	}
	if _, got, _, _ := a.poller.Display(quota.ItemCodex); !reflect.DeepEqual(got.Details, source.Details) || !got.ObservedAt.Equal(observed) {
		t.Fatalf("source mutated=%#v", got)
	}
}
