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

// resetLabels formats the panel and pill strings from one instant, so both
// surfaces must agree and the 30 s redraw must be the only clock needed.
func TestResetLabels(t *testing.T) {
	// Fixed local clock so the same-day/next-day branches are deterministic.
	now := time.Date(2026, time.October, 14, 15, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		name      string
		now       time.Time
		r         time.Time
		clock     bool
		wantPanel string
		wantPill  string
	}{
		{"countdown hours and minutes", now, now.Add(2*time.Hour + 14*time.Minute + 30*time.Second), false, "5h resets in 2h 14m", "2h14m"},
		{"countdown minutes only", now, now.Add(12*time.Minute + 30*time.Second), false, "5h resets in 12m", "12m"},
		{"countdown under a minute", now, now.Add(30 * time.Second), false, "5h resets in 0m", "0m"},
		{"clock same day", now, time.Date(2026, time.October, 14, 18, 42, 0, 0, time.Local), true, "5h resets 18:42", "18:42"},
		{"clock next day", now, time.Date(2026, time.October, 15, 2, 0, 0, 0, time.Local), true, "5h resets 15 Oct 02:00", "02:00"},
		{"expired countdown", now, now.Add(-time.Minute), false, "", ""},
		{"expired clock", now, now.Add(-time.Minute), true, "", ""},
		{"zero reset", now, time.Time{}, false, "", ""},
		// The old Detail-tail parse read "02:00" as same-day and dropped it as
		// already passed; from the instant it is simply 4h ahead.
		{"post-midnight countdown", time.Date(2026, time.October, 14, 22, 0, 0, 0, time.Local), time.Date(2026, time.October, 15, 2, 0, 0, 0, time.Local), false, "5h resets in 4h 0m", "4h0m"},
		{"post-midnight clock", time.Date(2026, time.October, 14, 22, 0, 0, 0, time.Local), time.Date(2026, time.October, 15, 2, 0, 0, 0, time.Local), true, "5h resets 15 Oct 02:00", "02:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panel, pill := resetLabels(tc.r, tc.now, tc.clock)
			if panel != tc.wantPanel || pill != tc.wantPill {
				t.Fatalf("resetLabels(%v, %v, clock=%v) = (%q, %q), want (%q, %q)", tc.r, tc.now, tc.clock, panel, pill, tc.wantPanel, tc.wantPill)
			}
		})
	}
}

func TestBuildViewStripsResetTail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The tail is the same instant as Reset5h, formatted the way readers write
	// it; only then is it the 5h tail the toggle replaces.
	reset := time.Now().Add(3 * time.Hour)
	detail := "5h 42% · week 31% · resets " + quota.FormatLocal(reset, "HH:mm")
	used := 42.0
	a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}}, resetClock: map[string]bool{}}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		return quota.Reading{Used: &used, Detail: detail, Reset5h: reset}
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
	// Countdown mode: stripped Detail plus the countdown strings is the only reset text.
	it := a.buildView().Items[0]
	if strings.Contains(it.Detail, "resets") {
		t.Fatalf("countdown Detail still carries reset tail: %q", it.Detail)
	}
	if it.Detail != "5h 42% · week 31%" {
		t.Fatalf("countdown Detail = %q, want %q", it.Detail, "5h 42% · week 31%")
	}
	if !strings.HasPrefix(it.Reset, "5h resets in ") {
		t.Fatalf("Reset = %q, want countdown form", it.Reset)
	}
	if it.PillReset == "" || strings.Contains(it.PillReset, " ") {
		t.Fatalf("PillReset = %q, want a compact countdown", it.PillReset)
	}
	if _, show, _, ok := a.poller.Display(quota.ItemCodex); !ok || show.Detail != detail {
		t.Fatalf("upstream Detail mutated: got %q, want %q", show.Detail, detail)
	}
	// Clock mode: same stripped Detail, the wall clock on both surfaces. The
	// pill keeps the plain HH:mm even when the panel adds the date.
	a.resetClock[quota.ItemCodex] = true
	flipped := a.buildView().Items[0]
	if strings.Contains(flipped.Detail, "resets") {
		t.Fatalf("clock Detail still carries reset tail: %q", flipped.Detail)
	}
	if flipped.Detail != "5h 42% · week 31%" {
		t.Fatalf("clock Detail = %q, want %q", flipped.Detail, "5h 42% · week 31%")
	}
	want := "5h resets " + quota.FormatLocal(reset, "HH:mm")
	if rl, nl := reset.In(time.Local), time.Now().In(time.Local); rl.Year() != nl.Year() || rl.YearDay() != nl.YearDay() {
		want = "5h resets " + quota.FormatLocal(reset, "d MMM HH:mm")
	}
	if flipped.Reset != want {
		t.Fatalf("clock Reset = %q, want %q", flipped.Reset, want)
	}
	if flipped.PillReset != quota.FormatLocal(reset, "HH:mm") {
		t.Fatalf("clock PillReset = %q, want %q", flipped.PillReset, quota.FormatLocal(reset, "HH:mm"))
	}
}

func TestBuildViewKeepsDetailWithoutReset(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	used := 42.0
	newApp := func(detail string) *app {
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
	// No reset tail and no Reset5h: Detail is byte-identical.
	it := newApp("5h 42%").buildView().Items[0]
	if it.Detail != "5h 42%" {
		t.Fatalf("reset-less Detail = %q, want byte-identical %q", it.Detail, "5h 42%")
	}
	if it.Reset != "" || it.PillReset != "" {
		t.Fatalf("reset-less row carries reset strings: %q %q", it.Reset, it.PillReset)
	}
	// A "2h"-labelled Codex primary still prints a resets tail but carries no
	// Reset5h; the tail is not the 5h window's and must survive untouched.
	withTail := "2h 42% · resets " + quota.FormatLocal(time.Now().Add(2*time.Hour), "HH:mm")
	it = newApp(withTail).buildView().Items[0]
	if it.Detail != withTail {
		t.Fatalf("non-5h tail Detail = %q, want byte-identical %q", it.Detail, withTail)
	}
	if it.Reset != "" || it.PillReset != "" {
		t.Fatalf("non-5h tail row carries reset strings: %q %q", it.Reset, it.PillReset)
	}
}

// A Detail tail that names a different instant than Reset5h — Claude shows the
// week window's reset while Reset5h tracks the five-hour one — stays visible;
// the 5h strings land beside it rather than eating it.
func TestBuildViewKeepsOtherWindowResetTail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CODEX_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	reset5h := time.Now().Add(90 * time.Minute)
	week := time.Now().Add(6 * 24 * time.Hour)
	detail := "5h 10% · week 95% · resets " + quota.FormatLocal(week, "d MMM HH:mm")
	used := 95.0
	a := &app{settings: settings{Visible: map[string]bool{quota.ItemCodex: true}}, resetClock: map[string]bool{}}
	a.poller = quota.NewPoller(func(string) quota.Reading {
		return quota.Reading{Used: &used, Detail: detail, Reset5h: reset5h}
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
	if it.Detail != detail {
		t.Fatalf("week-tail Detail = %q, want untouched %q", it.Detail, detail)
	}
	if !strings.HasPrefix(it.Reset, "5h resets in ") {
		t.Fatalf("Reset = %q, want countdown form", it.Reset)
	}
	if it.PillReset == "" {
		t.Fatal("PillReset is empty")
	}
}
