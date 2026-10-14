package main

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"agent-quota/internal/win"
	"agent-quota/quota"
	"agent-quota/widget"
	"golang.org/x/sys/windows"
)

// Reader completions must return while the window thread is busy, without
// touching app state until that thread processes their notification.
func TestPollCompletionQueuesWindowUpdate(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initial := time.Unix(1, 0)
	a := &app{settings: settings{Visible: map[string]bool{}}, checkedAt: initial, w: 140, h: 40, x: 200, y: 200}
	a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, a.changed)
	if err := win.Create("AgentQuota worker test", a.x, a.y, a); err != nil {
		t.Fatal(err)
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	cls, _ := windows.UTF16PtrFromString("AgentQuota")
	title, _ := windows.UTF16PtrFromString("AgentQuota worker test")
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)))
	defer func() {
		user32.NewProc("DestroyWindow").Call(hwnd)
		// DestroyWindow posts WM_QUIT. Drain it so this thread can be reused.
		var msg [8]uintptr
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	done := make(chan struct{})
	go func() { a.changed(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("quota completion blocked on the window thread")
	}
	if a.checkedAt != initial {
		t.Fatal("quota worker modified UI state before its queued update was dispatched")
	}
	user32.NewProc("PostMessageW").Call(hwnd, 0x111, menuQuit, 0)
	win.Run()
	if !a.checkedAt.After(initial) {
		t.Fatal("queued quota update was not applied by the window loop")
	}
}

func TestRelaunchRevealsExistingWidgetInsideWorkArea(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	work := win.WorkArea()
	a := &app{settings: settings{Visible: map[string]bool{}, HasPos: true, X: work.Min.X + 100, Y: work.Max.Y}, x: work.Min.X + 100, y: work.Max.Y, w: 152, h: 40, scale: 1}
	a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, a.changed)
	const title = "AgentQuota relaunch regression"
	if err := win.Create(title, a.x, a.y, a); err != nil {
		t.Fatal(err)
	}
	cls, _ := windows.UTF16PtrFromString("AgentQuota")
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)))
	defer func() {
		user32.NewProc("DestroyWindow").Call(hwnd)
		var msg [8]uintptr
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	a.redraw()
	if !win.NotifyExisting(title, menuReveal) {
		t.Fatal("relaunch notification did not find the existing widget")
	}
	user32.NewProc("PostMessageW").Call(hwnd, 0x111, menuQuit, 0)
	win.Run()
	if !a.open || !a.pinned {
		t.Fatal("second launch left the existing widget collapsed instead of revealing it")
	}
	var rect [4]int32
	user32.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	bounds := image.Rect(int(rect[0]), int(rect[1]), int(rect[2]), int(rect[3]))
	if !bounds.In(work) {
		t.Fatalf("revealed widget bounds %v overlap taskbar or leave work area %v", bounds, work)
	}
	same, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)))
	if same != hwnd {
		t.Fatal("relaunch replaced the window")
	}
}

func TestRefreshButtonForcesReadWithoutUnpinning(t *testing.T) {
	read := make(chan struct{}, 2)
	a := &app{settings: settings{Visible: map[string]bool{}}, open: true, pinned: true}
	a.poller = quota.NewPoller(func(string) quota.Reading { read <- struct{}{}; used := 42.0; return quota.Reading{Used: &used} }, func(id string) bool { return id == quota.ItemClaude }, func() {})
	a.poller.Refresh(false)
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("initial read did not start")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, ok := a.poller.Reading(quota.ItemClaude); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial read did not finish")
		}
		runtime.Gosched()
	}
	a.view = a.buildView()
	r := widget.RefreshBounds(a.view)
	a.OnPointerClick((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("Refresh did not force a fresh read")
	}
	if !a.pinned || !a.open {
		t.Fatal("Refresh unexpectedly toggled the inspector")
	}
}

func TestPlacementRecoversTaskbarPositionAndPreservesValidDrag(t *testing.T) {
	work := image.Rect(60, 40, 1860, 1120)
	for _, tc := range []struct {
		name  string
		s     settings
		w, h  int
		scale float32
		want  image.Point
	}{
		{"first launch", settings{}, 448, 40, 1, image.Pt(1388, 64)},
		{"saved on taskbar", settings{X: 400, Y: 1120, HasPos: true}, 448, 40, 1, image.Pt(1388, 64)},
		{"saved on removed monitor", settings{X: -1000, Y: 200, HasPos: true}, 448, 40, 1, image.Pt(1388, 64)},
		{"valid drag", settings{X: 300, Y: 700, HasPos: true}, 448, 40, 1, image.Pt(300, 700)},
		{"150 percent DPI", settings{}, 672, 60, 1.5, image.Pt(1152, 76)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x, y := initialPosition(tc.s, tc.w, tc.h, tc.scale, work)
			if image.Pt(x, y) != tc.want {
				t.Fatalf("got (%d,%d), want %v", x, y, tc.want)
			}
		})
	}
	x, y := clampWorkPos(-100, 1300, 448, 40, work)
	if x != 60 || y != 1080 {
		t.Fatalf("drag clamp = (%d,%d), want (60,1080)", x, y)
	}
}

func TestFormatCountdown(t *testing.T) {
	// Zero-hour waits omit hours so short resets read plainly.
	if got := formatCountdown(12 * time.Minute); got != "12m left" {
		t.Fatalf("12m countdown = %q, want %q", got, "12m left")
	}
	// Hour-plus waits keep both parts floored to the minute.
	if got := formatCountdown(3*time.Hour + 12*time.Minute + 50*time.Second); got != "3h 12m left" {
		t.Fatalf("3h12m countdown = %q, want %q", got, "3h 12m left")
	}
	// Day-plus waits collapse to days and remainder hours, minutes dropped.
	if got := formatCountdown(140*time.Hour + 5*time.Minute); got != "5d 20h left" {
		t.Fatalf("140h5m countdown = %q, want %q", got, "5d 20h left")
	}
	// Just under a day stays in hour form.
	if got := formatCountdown(23*time.Hour + 59*time.Minute); got != "23h 59m left" {
		t.Fatalf("23h59m countdown = %q, want %q", got, "23h 59m left")
	}
	// Passed or zero resets have no countdown; the caller falls back to clock.
	for _, d := range []time.Duration{0, -time.Minute, -time.Hour} {
		if got := formatCountdown(d); got != "" {
			t.Fatalf("passed countdown %v = %q, want empty", d, got)
		}
	}
}

func TestBuildViewStripsResetTail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Date-form tail stays in the future across midnight and year roll.
	future := time.Now().Add(3 * time.Hour)
	tail := "resets " + future.Format("2 Jan 15:04")
	detail := "5h 42% · week 31% · " + tail
	used := 42.0
	newCodexApp := func() *app {
		a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}}, resetClock: map[string]bool{}}
		a.poller = quota.NewPoller(func(string) quota.Reading {
			return quota.Reading{Used: &used, Detail: detail}
		}, func(id string) bool { return id == quota.ItemCodex }, func() {})
		a.poller.Refresh(true)
		deadline := time.Now().Add(2 * time.Second)
		for {
			if _, ok := a.poller.Reading(quota.ItemCodex); ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("codex read did not finish")
			}
			runtime.Gosched()
		}
		return a
	}
	// Countdown mode: stripped Detail plus the toggled countdown is the only reset text.
	a := newCodexApp()
	it := a.buildView().Items[0]
	if strings.Contains(it.Detail, "resets") {
		t.Fatalf("countdown Detail still carries reset tail: %q", it.Detail)
	}
	if it.Detail != "5h 42% · week 31%" {
		t.Fatalf("countdown Detail = %q, want %q", it.Detail, "5h 42% · week 31%")
	}
	if it.ResetClock != tail {
		t.Fatalf("ResetClock = %q, want %q", it.ResetClock, tail)
	}
	if it.ResetCountdown == "" {
		t.Fatal("ResetCountdown is empty for a usable reset")
	}
	if _, show, _, ok := a.poller.Display(quota.ItemCodex); !ok || show.Detail != detail {
		t.Fatalf("upstream Detail mutated: got %q, want %q", show.Detail, detail)
	}
	// Clock mode: same stripped Detail, only the wall clock is appended.
	a.resetClock[quota.ItemCodex] = true
	flipped := a.buildView().Items[0]
	if !flipped.ShowClock {
		t.Fatal("ShowClock not carried to the row")
	}
	if strings.Contains(flipped.Detail, "resets") {
		t.Fatalf("clock Detail still carries reset tail: %q", flipped.Detail)
	}
	if flipped.Detail != "5h 42% · week 31%" {
		t.Fatalf("clock Detail = %q, want %q", flipped.Detail, "5h 42% · week 31%")
	}
	if flipped.ResetClock != tail {
		t.Fatalf("clock ResetClock = %q, want %q", flipped.ResetClock, tail)
	}
}

func TestBuildViewKeepsDetailWithoutReset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := "5h 42%"
	used := 42.0
	a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}}, resetClock: map[string]bool{}}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		return quota.Reading{Used: &used, Detail: plain}
	}, func(id string) bool { return id == quota.ItemCodex }, func() {})
	a.poller.Refresh(true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := a.poller.Reading(quota.ItemCodex); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("codex read did not finish")
		}
		runtime.Gosched()
	}
	it := a.buildView().Items[0]
	if it.Detail != plain {
		t.Fatalf("reset-less Detail = %q, want byte-identical %q", it.Detail, plain)
	}
	if it.ResetCountdown != "" || it.ResetClock != "" {
		t.Fatalf("reset-less row carries reset strings: %q %q", it.ResetCountdown, it.ResetClock)
	}
}
