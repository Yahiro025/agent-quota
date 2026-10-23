package quota

import (
	"strings"
	"testing"
	"time"
)

func TestDecodeRequestAcceptsHelloRefreshAndShutdown(t *testing.T) {
	hello, err := DecodeRequest([]byte(`{"v":1,"op":"hello"}`))
	if err != nil || hello.Op != "hello" || hello.V != 1 {
		t.Fatalf("hello = %+v, %v", hello, err)
	}
	one, err := DecodeRequest([]byte(`{"v":1,"op":"refresh","id":"claude"}`))
	if err != nil || one.ID != "claude" {
		t.Fatalf("refresh = %+v, %v", one, err)
	}
	all, err := DecodeRequest([]byte(`{"v":1,"op":"refresh","id":"all"}`))
	if err != nil || all.ID != "all" {
		t.Fatalf("refresh all = %+v, %v", all, err)
	}
	bye, err := DecodeRequest([]byte(`{"v":1,"op":"shutdown"}`))
	if err != nil || bye.Op != "shutdown" {
		t.Fatalf("shutdown = %+v, %v", bye, err)
	}
	if _, err := DecodeRequest([]byte(`{"v":2,"op":"hello"}`)); err == nil {
		t.Fatal("version 2 accepted")
	}
	if _, err := DecodeRequest([]byte(`{"v":1,"op":"poke"}`)); err == nil {
		t.Fatal("unknown op accepted")
	}
	if _, err := DecodeRequest([]byte(`{"v":1,"op":"refresh"}`)); err == nil {
		t.Fatal("refresh without id accepted")
	}
	if _, err := DecodeRequest([]byte(`{"v":1,"op":"refresh","id":"nope"}`)); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func TestEncodeSnapshotOmitsZeroTimesAndKeepsZeroUsed(t *testing.T) {
	used := 0.0
	ev := ServiceEvent{V: 1, Op: "snapshot", Readings: []ServiceAgent{{
		ID: "claude", State: "ok", Busy: false, Used: &used,
		Reset5h: time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
	}}}
	raw, err := EncodeEvent(ev)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"used":0`) || strings.Contains(text, "observedAt") {
		t.Fatalf("encoding = %s", text)
	}
	if strings.Contains(text, "\n") {
		t.Fatal("encoder added a newline")
	}
}

func TestSnapshotCarriesHelpNotSecrets(t *testing.T) {
	changed := make(chan struct{}, 4)
	p := NewPoller(
		func(id string) Reading { return Fail("Sign-in expired.") },
		func(id string) bool { return true },
		func() { changed <- struct{}{} },
	)
	s := NewService(p)
	s.Handle(`{"v":1,"op":"refresh","id":"devin"}`)
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("no change was said")
	}

	raw, err := EncodeEvent(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	line := string(raw)
	// A failed reading sends both the reason and its recovery hint: the
	// client's detail block needs the pair to render the review-focus state.
	if !strings.Contains(line, `"err":"Sign-in expired."`) {
		t.Fatalf("no err in %s", line)
	}
	if !strings.Contains(line, `"help":"Open Devin to renew its sign-in, then choose Refresh."`) {
		t.Fatalf("no help in %s", line)
	}
	for _, secret := range []string{"token", "apiKey", "password", "credential"} {
		if strings.Contains(line, secret) {
			t.Fatalf("snapshot leaks %q: %s", secret, line)
		}
	}
}
