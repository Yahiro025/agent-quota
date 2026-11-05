package win

import (
	"os"
	"runtime"
	"slices"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestLiveAgentKeyboardHookDelivery(t *testing.T) {
	if os.Getenv("AGENTQUOTA_KEYBOARD_LIVE_TEST") != "1" {
		t.Skip("opt-in native keyboard fixture")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	dpi := user32.NewProc("SetThreadDpiAwarenessContext")
	previous, _, _ := dpi.Call(^uintptr(3))
	defer dpi.Call(previous)
	const token uintptr = 0x41514b4254455354
	var received []int
	barriers := 0
	receiver := windows.NewCallback(func(code, message uintptr, data unsafe.Pointer) uintptr {
		if int32(code) >= 0 && data != nil {
			event := (*struct {
				vk, scan, flags, time uint32
				extra                 uintptr
			})(data)
			if event.extra == token {
				vk := int(event.vk)
				if message == 0x101 {
					vk = -vk
				}
				if event.vk == 0x87 {
					if vk < 0 {
						barriers++
					}
				} else {
					received = append(received, vk)
				}
				return 1
			}
		}
		r, _, _ := pNextKeyHook.Call(0, code, message, uintptr(data))
		return r
	})
	module, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	hook, _, err := pSetKeyboardHook.Call(13, receiver, module, 0)
	if hook == 0 {
		t.Fatalf("test receiver hook: %v", err)
	}
	defer pUnhookKeyboard.Call(hook)
	foreground := user32.NewProc("GetForegroundWindow")
	before, _, _ := foreground.Call()
	x, y := cursorPos()
	callback := &keyboardTestCallback{}
	if err := Create("AgentQuota native key delivery fixture", x-16, y-16, callback); err != nil {
		t.Fatal(err)
	}
	defer func() {
		user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
		hwnd = 0
		var message [8]uintptr
		pPeekTaskbarMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0x12, 0x12, 1)
	}()
	if activeAgentKeyboard.Load() == nil {
		t.Fatal("production keyboard hook is unavailable")
	}
	EnableAgentNavigation(true)
	pixels := make([]byte, 32*32*4)
	for i := 3; i < len(pixels); i += 4 {
		pixels[i] = 65
	}
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			pixels[(y*32+x)*4+3] = 255
		}
	}
	if err := Flush(pixels, 32, 32); err != nil {
		t.Fatal(err)
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
	send := func(keys ...int) {
		t.Helper()
		wantBarrier := barriers + 1
		keys = append(keys, 0x87, -0x87)
		for _, key := range keys {
			var input struct {
				kind        uint32
				padding     uint32
				vk, scan    uint16
				flags, time uint32
				extra       uintptr
				unionPad    [8]byte
			}
			input.kind, input.extra = 1, token
			if key < 0 {
				key, input.flags = -key, 2
			}
			input.vk = uint16(key)
			if key == 0x25 || key == 0x27 {
				input.flags |= 1
			}
			count, _, err := user32.NewProc("SendInput").Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
			if count != 1 {
				t.Fatalf("SendInput fixture: %v", err)
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			pump()
			if barriers == wantBarrier {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("native key fixture did not reach its receiver barrier")
			}
			user32.NewProc("MsgWaitForMultipleObjects").Call(0, 0, 0, 10, 0x4ff)
		}
	}
	send(0x27, 0x27, -0x27)
	Resize(x+100, y+100, 32, 32)
	send(0x25)
	Resize(x-16, y-16, 32, 32)
	send(0x25, -0x25)
	send(0x25)
	Resize(x+100, y+100, 32, 32)
	send(0x25, -0x25)
	Resize(x-2, y-2, 32, 32)
	send(0x27, -0x27)
	EnableAgentNavigation(false)
	Resize(x-16, y-16, 32, 32)
	send(0x27, -0x27)
	if !slices.Equal(callback.steps, []int{1, -1}) {
		t.Fatalf("hook navigation=%v, want [1 -1]", callback.steps)
	}
	if !slices.Equal(received, []int{0x25, 0x25, -0x25, 0x27, -0x27, 0x27, -0x27}) {
		t.Fatalf("receiver got %v, expected balanced passed keys only", received)
	}
	after, _, _ := foreground.Call()
	if after != before {
		t.Fatal("native keyboard fixture changed the foreground window")
	}
	t.Log("Windows invoked the production hook; captured repeat/release stayed suppressed; passed hold-enter, shadow and disabled sequences stayed balanced; foreground unchanged")
}
