package main

import (
	"encoding/json"
	"image"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"agent-quota/internal/win"
	"agent-quota/quota"
	"agent-quota/widget"
	"golang.org/x/sys/windows"
)

// Opt-in because this test attaches to the real enabled deskband. The normal
// app must be stopped; fixture readings never invoke a provider or credentials.
func TestLiveTaskbarFrameOwnership(t *testing.T) {
	if os.Getenv("AGENTQUOTA_LIVE_TEST") != "1" {
		t.Skip("requires an enabled host and the normal app stopped")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	win.EnableDPIAwareness()
	var dual atomic.Bool
	a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}, Selected: quota.ItemCodex, Compact: true, Docked: true, HasPos: true, X: 900, Y: 600}, scale: win.DPIScale(), x: 900, y: 600}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		used := 42.0
		windows := []quota.Window{{Label: "5h", Used: 42}}
		if dual.Load() {
			windows = append(windows, quota.Window{Label: "week", Used: 16})
		}
		return quota.Reading{Used: &used, Windows: windows}
	}, func(id string) bool { return id == quota.ItemCodex }, a.changed)
	a.w, a.h = widget.Size(a.buildView())
	const title = "AgentQuota live ownership regression"
	if err := win.Create(title, a.x, a.y, a); err != nil {
		t.Fatal(err)
	}
	user := windows.NewLazySystemDLL("user32.dll")
	class, _ := windows.UTF16PtrFromString("AgentQuota")
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := user.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
	peek := user.NewProc("PeekMessageW")
	dispatch := user.NewProc("DispatchMessageW")
	translate := user.NewProc("TranslateMessage")
	pump := func() {
		var message [8]uintptr
		for {
			r, _, _ := peek.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			translate.Call(uintptr(unsafe.Pointer(&message[0])))
			dispatch.Call(uintptr(unsafe.Pointer(&message[0])))
		}
	}
	defer func() {
		if a.taskbar != nil {
			a.taskbar.Disconnect()
		}
		user.NewProc("DestroyWindow").Call(hwnd)
		pump()
	}()
	wait := func(label string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			pump()
			if ready() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s timed out: docked=%v focus=%v native=%+v", label, a.docked, a.view.Focus, a.taskbar.Snapshot())
			}
			user.NewProc("MsgWaitForMultipleObjects").Call(0, 0, 0, 10, 0x04ff)
		}
	}
	visible := func() bool { r, _, _ := user.NewProc("IsWindowVisible").Call(hwnd); return r != 0 }
	a.taskbar = win.NewTaskbarBridge()
	if !a.taskbar.Snapshot().Available {
		t.Fatal("matching native host is unavailable")
	}
	a.redraw()
	a.poller.Refresh(true)
	wait("initial native fit", func() bool {
		return a.docked && !visible() && len(a.view.Items) == 1 && len(a.view.Items[0].Windows) == 1
	})
	var records []map[string]any
	for i := 0; i < 12; i++ {
		want := 2
		if i%2 == 1 {
			want = 1
		}
		dual.Store(want == 2)
		a.poller.Refresh(true)
		wait("resized frame ownership", func() bool {
			return !a.poller.Busy(quota.ItemCodex) && len(a.view.Items) == 1 && len(a.view.Items[0].Windows) == want && a.docked && !visible() && a.taskbar.Snapshot().Fits
		})
		s := a.taskbar.Snapshot()
		records = append(records, map[string]any{"pools": want, "slot": s.Readout, "native_fits": s.Fits, "floating_visible": visible()})
	}
	a.settings.Docked = false
	a.redraw()
	wait("floating handoff", func() bool { return visible() && !a.docked && a.taskbar.Snapshot().Readout.Dx() == 0 })
	work := win.WorkArea()
	pill := widget.PillBounds(a.view)
	if !image.Rect(a.x-pill.Min.X, a.y-pill.Min.Y, a.x-pill.Min.X+a.w, a.y-pill.Min.Y+a.h).In(work) {
		t.Fatal("floating fallback is outside the work area")
	}
	a.settings.Docked = true
	a.redraw()
	wait("dock handoff", func() bool { return a.docked && !visible() && a.taskbar.Snapshot().Fits })
	if path := os.Getenv("AGENTQUOTA_DRAG_EVIDENCE"); path != "" {
		checkLiveTaskbarDrag(t, a, pump, path)
	}
	data, _ := json.MarshalIndent(records, "", "  ")
	t.Log(string(data))
	if path := os.Getenv("AGENTQUOTA_LIVE_EVIDENCE"); path != "" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
