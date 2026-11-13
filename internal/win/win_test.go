package win

import (
	"image"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type testCallback struct{}

func (testCallback) Paint(int, int) []byte { return nil }
func (testCallback) OnMove(int, int)       {}
func (testCallback) OnClick()              {}
func (testCallback) OnHover(bool)          {}
func (testCallback) OnMenu(int)            {}
func (testCallback) Menu() []MenuItem      { return nil }
func (testCallback) Size() (int, int)      { return 32, 32 }

// A successfully uploaded opaque surface must receive hits and appear on screen.
// Pure Render tests cannot catch incorrect arguments to UpdateLayeredWindow.
func TestFlushPresentsOpaquePixels(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// GetPixel uses physical coordinates; keep the test window in that space
	// even when the desktop scales DPI-unaware applications.
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previousDPI, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previousDPI)
	if err := Create("AgentQuota paint test", 100, 100, testCallback{}); err != nil {
		t.Fatal(err)
	}
	if hwnd == 0 {
		t.Fatal("Create returned without a window")
	}
	defer func() {
		user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
		hwnd = 0
	}()
	pixels := make([]byte, 32*32*4)
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 0x33, 0x66, 0xCC, 0xFF
	}
	if err := Flush(pixels, 32, 32); err != nil {
		t.Fatal(err)
	}
	windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmFlush").Call()
	var p = point{116, 116}
	// WindowFromPoint takes POINT by value on Windows amd64.
	hit, _, _ := user32.NewProc("WindowFromPoint").Call(*(*uintptr)(unsafe.Pointer(&p)))
	if hit != uintptr(hwnd) {
		t.Fatalf("opaque surface is missing: hit window %#x, want %#x", hit, hwnd)
	}
	dc, _, _ := user32.NewProc("GetDC").Call(0)
	defer user32.NewProc("ReleaseDC").Call(0, dc)
	color, _, _ := gdi32.NewProc("GetPixel").Call(dc, 116, 116)
	if color != 0x3366CC {
		t.Fatalf("screen pixel = %#x, want opaque COLORREF 0x3366cc", color)
	}
}

// Force another goroutine to occupy the current thread while the UI yields.
// The production lifecycle must keep the window's owner thread for its loop.
func TestLoopRetainsWindowThread(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	var owner, after uint32
	err := Loop(func() error {
		if err := Create("AgentQuota thread test", 100, 100, testCallback{}); err != nil {
			return err
		}
		defer func() {
			user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
			hwnd = 0
		}()
		owner = windows.GetCurrentThreadId()
		ready := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			close(ready)
			<-release
			close(finished)
		}()
		<-ready
		time.Sleep(time.Millisecond)
		after = windows.GetCurrentThreadId()
		close(release)
		<-finished
		// Stop before pumping: the test only probes ownership across a yield.
		return windows.ERROR_CANCELLED
	})
	if err != windows.ERROR_CANCELLED {
		t.Fatalf("window setup: %v", err)
	}
	if after != owner {
		t.Fatalf("UI moved from window owner thread %d to thread %d", owner, after)
	}
}

// Probe windows must not run the app's message handler: the callback is nil in
// a bare test process.
var probeWindowProc = windows.NewCallback(func(h, msg, w, l uintptr) uintptr {
	r, _, _ := pDefWindowProc.Call(h, msg, w, l)
	return r
})

// probeWindow creates and shows a real top-level window exactly covering r.
// WS_EX_LAYERED with zero alpha keeps the desktop visually untouched while the
// window is still genuinely visible and covering the monitor.
func probeWindow(t *testing.T, class string, style, exstyle uintptr, r image.Rectangle) uintptr {
	t.Helper()
	cls, _ := windows.UTF16PtrFromString(class)
	var kc wndClassEx
	kc.size = uint32(unsafe.Sizeof(kc))
	kc.proc = probeWindowProc
	kc.className = cls
	pRegisterClass.Call(uintptr(unsafe.Pointer(&kc)))
	h, _, err := pCreateWindowEx.Call(exstyle|wsExLayered, uintptr(unsafe.Pointer(cls)), 0, style,
		uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()), 0, 0, 0, 0)
	runtime.KeepAlive(cls)
	if h == 0 {
		t.Fatalf("create %q window: %v", class, err)
	}
	user32.NewProc("SetLayeredWindowAttributes").Call(h, 0, 0, 2) // LWA_ALPHA
	pShowWindow.Call(h, swShowNoactivate)
	return h
}

// A captionless popup covering the primary monitor is fullscreen; a titled
// window of the same size is an ordinary maximised app on an auto-hide
// taskbar, and a tool window is a same-band appbar such as MyDockFinder.
func TestFullscreenForDistinguishesBorderlessFullscreenFromTitledAndToolWindows(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	// A probe must not be excused as the app's own window.
	own := hwnd
	hwnd = 0
	defer func() { hwnd = own }()
	mon := MonitorBounds()
	if mon.Empty() {
		t.Fatal("no primary monitor bounds")
	}
	for _, tc := range []struct {
		name    string
		class   string
		style   uintptr
		exstyle uintptr
		want    bool
	}{
		{"borderless fullscreen", "AgentQuotaFullscreenProbe", wsPopup, 0, true},
		{"titled window", "AgentQuotaTitledProbe", 0x00CF0000, 0, false}, // WS_OVERLAPPEDWINDOW
		{"tool window", "AgentQuotaToolProbe", wsPopup, wsExToolwindow | wsExNoactivate, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := probeWindow(t, tc.class, tc.style, tc.exstyle, mon)
			defer func() {
				user32.NewProc("DestroyWindow").Call(probe)
			}()
			if got := FullscreenFor(probe); got != tc.want {
				t.Fatalf("FullscreenFor(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
