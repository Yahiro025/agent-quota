package win

import (
	"os"
	"runtime"
	"slices"
	"testing"
	"time"
	"unsafe"
)

func TestArrowOwnershipKeepsPressRepeatAndReleaseTogether(t *testing.T) {
	var keys arrowOwnership
	for _, event := range []struct {
		name                    string
		vk                      uint32
		down, eligible, consume bool
		step                    int
	}{
		{"right press", 0x27, true, true, true, 1},
		{"right repeat", 0x27, true, true, true, 0},
		{"leave while held", 0x27, true, false, true, 0},
		{"release outside", 0x27, false, false, true, 0},
		{"press outside", 0x27, true, false, false, 0},
		{"enter while held", 0x27, true, true, false, 0},
		{"passed release", 0x27, false, true, false, 0},
		{"left press", 0x25, true, true, true, -1},
		{"left release", 0x25, false, false, true, 0},
		{"modified press", 0x25, true, false, false, 0},
		{"modifier released while held", 0x25, true, true, false, 0},
		{"modified release", 0x25, false, true, false, 0},
		{"unrelated key", 0x41, true, true, false, 0},
		{"new right press", 0x27, true, true, true, 1},
	} {
		t.Run(event.name, func(t *testing.T) {
			consume, step := keys.route(event.vk, event.down, event.eligible)
			if consume != event.consume || step != event.step {
				t.Fatalf("consume=%v step=%d, want %v %d", consume, step, event.consume, event.step)
			}
		})
	}
}

type keyboardTestCallback struct {
	testCallback
	steps []int
}

func (c *keyboardTestCallback) OnAgentNavigate(step int) { c.steps = append(c.steps, step) }
func (*keyboardTestCallback) ContainsPoint(x, y int) bool {
	return x >= 8 && x < 24 && y >= 8 && y < 24
}

func TestAgentKeyboardNativeScopeLifecycleAndForeground(t *testing.T) {
	if os.Getenv("AGENTQUOTA_KEYBOARD_LIVE_TEST") != "1" {
		t.Skip("opt-in native keyboard fixture")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	foreground := user32.NewProc("GetForegroundWindow")
	before, _, _ := foreground.Call()
	x, y := cursorPos()
	callback := &keyboardTestCallback{}
	if err := Create("AgentQuota keyboard policy test", x-16, y-16, callback); err != nil {
		t.Fatal(err)
	}
	k := activeAgentKeyboard.Load()
	if k == nil || k.hook.Load() == 0 || prop(uintptr(hwnd), "AQ.AgentKeys") != 1 {
		t.Fatal("native keyboard hook did not install")
	}
	pump := func() {
		var message [8]uintptr
		for {
			ok, _, _ := pPeekTaskbarMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0, 1)
			if ok == 0 {
				return
			}
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&message[0])))
		}
	}
	defer func() {
		user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
		hwnd = 0
		deadline := time.Now().Add(3 * time.Second)
		for {
			pump()
			select {
			case <-k.done:
				if activeAgentKeyboard.Load() != nil || k.hook.Load() != 0 {
					t.Error("keyboard hook survived window destruction")
				}
				return
			default:
				if time.Now().After(deadline) {
					t.Error("keyboard thread did not exit")
					return
				}
				runtime.Gosched()
			}
		}
	}()
	pixels := make([]byte, 32*32*4)
	for i := 3; i < len(pixels); i += 4 {
		pixels[i] = 65
	}
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			pixels[(y*32+x)*4+3] = 255
		}
	}
	EnableAgentNavigation(true)
	if err := Flush(pixels, 32, 32); err != nil {
		t.Fatal(err)
	}
	eligible := func() bool {
		result := make(chan bool, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			result <- k.eligible()
		}()
		for {
			pump()
			select {
			case result := <-result:
				return result
			default:
				runtime.Gosched()
			}
		}
	}
	if !eligible() {
		t.Fatal("open widget under pointer did not accept arrow scope")
	}
	post := func(scope uint64, vk uintptr) {
		pPostMessage.Call(uintptr(hwnd), agentNavigateMessage, uintptr(scope), vk)
		pump()
	}
	scope := k.scope.Load()
	post(scope, 0x27)
	post(scope, 0x25)
	SetVisible(false)
	post(scope, 0x27)
	if eligible() {
		t.Fatal("hidden widget accepts arrows")
	}
	SetVisible(true)
	EnableAgentNavigation(true)
	post(scope, 0x27)
	post(k.scope.Load(), 0x27)
	k.paused.Store(true)
	post(k.scope.Load(), 0x27)
	if eligible() {
		t.Fatal("native menu pause accepts arrows")
	}
	k.paused.Store(false)
	Resize(x-2, y-2, 32, 32)
	if eligible() {
		t.Fatal("shadow-only hit region accepts arrows")
	}
	Resize(x+100, y+100, 32, 32)
	if eligible() {
		t.Fatal("pointer outside widget accepts arrows")
	}
	if !slices.Equal(callback.steps, []int{1, -1, 1}) {
		t.Fatalf("native navigation steps=%v, want [1 -1 1]", callback.steps)
	}
	after, _, _ := foreground.Call()
	if after != before {
		t.Fatal("keyboard navigation changed foreground window")
	}
}
