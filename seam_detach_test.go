package main

import (
	"image"
	"runtime"
	"testing"
	"unsafe"

	"agent-quota/internal/win"
	"agent-quota/quota"
	"agent-quota/widget"
	"golang.org/x/sys/windows"
)

// A notch's full upper attachment must not survive a move into the desktop.
func TestSeamPositionSelectsAttachedOrFloatingContour(t *testing.T) {
	// Seam attaches to the monitor's top edge, which the work area may not reach
	// when a top appbar reserves y = 0..30.
	work := win.SeamWorkArea()
	a := &app{settings: settings{Visible: map[string]bool{}, Compact: true}, scale: 1}
	a.x, a.y = (work.Min.X+work.Max.X)/2-76, work.Min.Y
	for _, tc := range []struct {
		name   string
		dx, dy int
		docked bool
	}{
		{"top centre", 0, 0, true},
		{"desktop", 0, 200, false},
		{"top edge after horizontal drag", 100, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.x, a.y = (work.Min.X+work.Max.X)/2-76+tc.dx, work.Min.Y+tc.dy
			v := a.buildView()
			r := widget.PillBounds(v)
			p := r.Min.Add(image.Pt(8, 1))
			alpha := widget.Render(v).RGBAAt(p.X, p.Y).A
			if tc.docked && alpha != 255 {
				t.Fatalf("attached upper connection is not opaque: alpha=%d", alpha)
			}
			if !tc.docked && alpha >= 128 {
				t.Fatalf("floating rail retains a notch attachment corner: alpha=%d", alpha)
			}
			if !v.Compact || !a.settings.Compact {
				t.Fatal("movement changed the user's compact preference")
			}
		})
	}
}

func TestSeamDragChangesContourWithoutMovingItsAnchor(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	work := win.SeamWorkArea()
	a := &app{settings: settings{Visible: map[string]bool{}, Compact: true, Selected: quota.ItemAntigravity}, scale: 1}
	a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, func() {})
	a.x, a.y = (work.Min.X+work.Max.X)/2-76, work.Min.Y
	a.w, a.h = widget.Size(a.buildView())
	const title = "AgentQuota Seam detach regression"
	if err := win.Create(title, a.x, a.y, a); err != nil {
		t.Fatal(err)
	}
	class, _ := windows.UTF16PtrFromString("AgentQuota")
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
	defer func() {
		user32.NewProc("ReleaseCapture").Call()
		user32.NewProc("DestroyWindow").Call(hwnd)
		var msg [8]uintptr
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	a.redraw()
	startX, startY := a.x, a.y
	pill := widget.PillBounds(a.view)
	origin := image.Pt(startX-pill.Min.X, startY-pill.Min.Y)
	p := pill.Min.Add(image.Pt(30, 20))
	user32.NewProc("SendMessageW").Call(hwnd, 0x201, 1, uintptr(p.X|p.Y<<16))
	for _, delta := range []image.Point{{60, 200}, {100, 250}, {0, 0}} {
		a.OnMove(origin.X+delta.X, origin.Y+delta.Y)
		if a.x != startX+delta.X || a.y != startY+delta.Y {
			t.Fatalf("drag %v shifted visible anchor to (%d,%d), want (%d,%d)", delta, a.x, a.y, startX+delta.X, startY+delta.Y)
		}
		r := widget.PillBounds(a.view)
		p := r.Min.Add(image.Pt(8, 1))
		alpha := widget.Render(a.view).RGBAAt(p.X, p.Y).A
		if delta.Y > 0 && alpha >= 128 {
			t.Fatalf("drag did not repaint the detached contour: alpha=%d", alpha)
		}
		if delta.Y == 0 && alpha != 255 {
			t.Fatalf("returning to the notch did not restore attachment: alpha=%d", alpha)
		}
	}
	// Reopening must retain the detached rail's anchor as the overview grows.
	a.OnMove(origin.X+60, origin.Y+200)
	want := image.Pt(startX+60, startY+200)
	for _, open := range []bool{true, false} {
		a.OnHover(open)
		if a.open != open || !a.view.Floating || image.Pt(a.x, a.y) != want {
			t.Fatalf("hover=%v lost floating state or anchor: open=%v floating=%v pos=(%d,%d)", open, a.open, a.view.Floating, a.x, a.y)
		}
	}
	a.reveal()
	if !a.open || !a.pinned || !a.view.Floating || image.Pt(a.x, a.y) != want {
		t.Fatal("second-launch reveal moved or reattached the floating rail")
	}
	close := widget.CloseBounds(a.view)
	a.OnPointerClick((close.Min.X+close.Max.X)/2, (close.Min.Y+close.Max.Y)/2)
	if a.open || a.pinned || !a.view.Floating || image.Pt(a.x, a.y) != want {
		t.Fatal("closing the floating overview moved or reattached its rail")
	}
	a.OnDragEnd()
	saved := loadSettings()
	if !saved.HasPos || image.Pt(saved.X, saved.Y) != want || !saved.Compact || saved.Selected != quota.ItemAntigravity {
		t.Fatalf("saved drag lost the position or preferences: %+v", saved)
	}
}

// The 2 s maintenance tick owns fullscreen suppression. It must hide Seam while
// a fullscreen app covers the primary monitor, leave it hidden while the state
// persists, and restore it without stealing focus when the app exits.
func TestFullscreenTickHidesAndRestoresSeamWithoutActivation(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	fullscreen := false
	a := &app{
		settings:        settings{Visible: map[string]bool{}, Compact: true, Selected: quota.ItemAntigravity},
		scale:           1,
		fullscreenCheck: func() bool { return fullscreen },
	}
	a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, func() {})
	seam := win.SeamWorkArea()
	a.x, a.y = (seam.Min.X+seam.Max.X)/2-76, seam.Min.Y
	a.w, a.h = widget.Size(a.buildView())
	const title = "AgentQuota fullscreen tick regression"
	if err := win.Create(title, a.x, a.y, a); err != nil {
		t.Fatal(err)
	}
	class, _ := windows.UTF16PtrFromString("AgentQuota")
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
	defer func() {
		user32.NewProc("DestroyWindow").Call(hwnd)
		var msg [8]uintptr
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	a.open, a.hovered = true, widget.Target{Control: widget.PinControl}
	a.redraw()
	fullscreen = true
	a.OnMenu(104)
	if !a.fullscreen || a.open || a.hovered != (widget.Target{}) {
		t.Fatalf("fullscreen tick kept the overview: fullscreen=%v open=%v hovered=%+v", a.fullscreen, a.open, a.hovered)
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd); visible != 0 {
		t.Fatal("fullscreen tick left Seam painted over a fullscreen app")
	}
	// Steady state: the tick must not resurrect the deliberately hidden window.
	a.OnMenu(104)
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd); visible != 0 {
		t.Fatal("maintenance tick showed a hidden window")
	}
	fullscreen = false
	a.OnMenu(104)
	if a.fullscreen {
		t.Fatal("fullscreen exit was not noticed")
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd); visible == 0 {
		t.Fatal("fullscreen exit did not restore Seam")
	}
	if fg, _, _ := user32.NewProc("GetForegroundWindow").Call(); fg == hwnd {
		t.Fatal("restoring Seam activated the focus-free window")
	}
}
