package win

import (
	"runtime"
	"testing"
	"unsafe"
)

type orbitHitCallback struct{ testCallback }

func (orbitHitCallback) ContainsPoint(x, y int) bool { return x >= 8 && x < 24 && y >= 8 && y < 24 }

func TestOrbitWindowKeepsFocusAndRejectsShadowHits(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	foreground := user32.NewProc("GetForegroundWindow")
	before, _, _ := foreground.Call()
	if err := Create("AgentQuota Orbit policy test", 100, 100, orbitHitCallback{}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
		hwnd = 0
		var msg [8]uintptr
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	pixels := make([]byte, 32*32*4)
	for i := 3; i < len(pixels); i += 4 {
		pixels[i] = 255
	}
	if err := Flush(pixels, 32, 32); err != nil {
		t.Fatal(err)
	}
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(uintptr(hwnd), ^uintptr(19))
	want := uintptr(wsExLayered | wsExTopmost | wsExToolwindow | wsExNoactivate)
	if style&want != want {
		t.Fatalf("window styles = %#x, missing %#x", style, want)
	}
	if got := wndProc(hwnd, wmMouseactivate, 0, 0); got != maNoactivate {
		t.Fatalf("mouse activation = %d", got)
	}
	point := point{102, 102}
	hit, _, _ := user32.NewProc("WindowFromPoint").Call(*(*uintptr)(unsafe.Pointer(&point)))
	if hit == uintptr(hwnd) {
		t.Fatal("shadow-only area intercepts pointer input")
	}
	after, _, _ := foreground.Call()
	if before != after {
		t.Fatal("creating or painting Orbit changed the foreground window")
	}
}
