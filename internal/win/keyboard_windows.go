package win

import (
	"fmt"
	"image"
	"runtime"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const agentNavigateMessage = 0x8000 + 61

type arrowKeyState uint8

const (
	arrowUp arrowKeyState = iota
	arrowPassed
	arrowCaptured
)

type arrowOwnership [2]arrowKeyState

func (keys *arrowOwnership) route(vk uint32, down, eligible bool) (bool, int) {
	index, step := 0, -1
	if vk == 0x27 {
		index, step = 1, 1
	} else if vk != 0x25 {
		return false, 0
	}
	state := &keys[index]
	if !down {
		captured := *state == arrowCaptured
		*state = arrowUp
		return captured, 0
	}
	if *state != arrowUp {
		return *state == arrowCaptured, 0
	}
	*state = arrowPassed
	if eligible {
		*state = arrowCaptured
		return true, step
	}
	return false, 0
}

type agentKeyboard struct {
	window      uintptr
	hook        atomic.Uintptr
	thread      uint32
	scope, next atomic.Uint64
	paused      atomic.Bool
	mask        atomic.Pointer[image.Alpha]
	keys        arrowOwnership
	done        chan struct{}
}

var activeAgentKeyboard atomic.Pointer[agentKeyboard]

var (
	pSetKeyboardHook = user32.NewProc("SetWindowsHookExW")
	pUnhookKeyboard  = user32.NewProc("UnhookWindowsHookEx")
	pNextKeyHook     = user32.NewProc("CallNextHookEx")
	pAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
	pWindowAtPoint   = user32.NewProc("WindowFromPoint")
	pWindowVisible   = user32.NewProc("IsWindowVisible")
	pPostThread      = user32.NewProc("PostThreadMessageW")
	pSetAgentProp    = user32.NewProc("SetPropW")
	pRemoveAgentProp = user32.NewProc("RemovePropW")
	agentKeysProp    = windows.StringToUTF16Ptr("AQ.AgentKeys")
)

func keyHeld(vk uintptr) bool {
	r, _, _ := pAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

func (k *agentKeyboard) eligible() bool {
	if k.scope.Load() == 0 || k.paused.Load() {
		return false
	}
	for _, vk := range [...]uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c, 0x01} {
		if keyHeld(vk) {
			return false
		}
	}
	visible, _, _ := pWindowVisible.Call(k.window)
	x, y := cursorPos()
	bounds := rectOf(k.window)
	mask := k.mask.Load()
	if visible == 0 || mask == nil || mask.AlphaAt(x-bounds.Min.X, y-bounds.Min.Y).A < 128 {
		return false
	}
	p := point{int32(x), int32(y)}
	hit, _, _ := pWindowAtPoint.Call(*(*uintptr)(unsafe.Pointer(&p)))
	return hit == k.window
}

func publishAgentHitMask(bgra []byte, w, h int) {
	if k := activeAgentKeyboard.Load(); k != nil && k.scope.Load() != 0 {
		mask := image.NewAlpha(image.Rect(0, 0, w, h))
		for i := range mask.Pix {
			mask.Pix[i] = bgra[i*4+3]
		}
		k.mask.Store(mask)
	}
}

func (k *agentKeyboard) hookProc(code, message uintptr, data unsafe.Pointer) uintptr {
	if int32(code) >= 0 && data != nil {
		vk := *(*uint32)(data)
		if vk == 0x25 || vk == 0x27 {
			down := message == 0x100 || message == 0x104
			up := message == 0x101 || message == 0x105
			if down || up {
				scope := k.scope.Load()
				consume, step := k.keys.route(vk, down, down && k.eligible())
				if step != 0 {
					pPostMessage.Call(k.window, agentNavigateMessage, uintptr(scope), uintptr(vk))
				}
				if consume {
					return 1
				}
			}
		}
	}
	r, _, _ := pNextKeyHook.Call(0, code, message, uintptr(data))
	return r
}

func startAgentKeyboard(window uintptr) error {
	stopAgentKeyboard()
	k := &agentKeyboard{window: window, done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(k.done)
		var message [8]uintptr
		pPeekTaskbarMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0, 0)
		thread, _, _ := kernel32.NewProc("GetCurrentThreadId").Call()
		k.thread = uint32(thread)
		for i, vk := range [...]uintptr{0x25, 0x27} {
			if keyHeld(vk) {
				k.keys[i] = arrowPassed
			}
		}
		module, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
		callback := windows.NewCallback(k.hookProc)
		hook, _, err := pSetKeyboardHook.Call(13, callback, module, 0)
		if hook == 0 {
			ready <- fmt.Errorf("agent arrow keys: %w", err)
			return
		}
		k.hook.Store(hook)
		defer k.unhook()
		ready <- nil
		for {
			r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&message[0])), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&message[0])))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&message[0])))
		}
	}()
	if err := <-ready; err != nil {
		return err
	}
	activeAgentKeyboard.Store(k)
	pSetAgentProp.Call(window, uintptr(unsafe.Pointer(agentKeysProp)), 1)
	return nil
}

func stopAgentKeyboard() {
	if k := activeAgentKeyboard.Swap(nil); k != nil {
		pRemoveAgentProp.Call(k.window, uintptr(unsafe.Pointer(agentKeysProp)))
		k.scope.Store(0)
		k.unhook()
		pPostThread.Call(uintptr(k.thread), 0x12, 0, 0)
	}
}

func (k *agentKeyboard) unhook() {
	if hook := k.hook.Swap(0); hook != 0 {
		pUnhookKeyboard.Call(hook)
	}
}

func EnableAgentNavigation(enabled bool) {
	if k := activeAgentKeyboard.Load(); k != nil {
		if !enabled {
			k.scope.Store(0)
		} else if k.scope.Load() == 0 {
			k.scope.Store(k.next.Add(1))
		}
	}
}

func handleAgentNavigation(scope, vk uintptr) {
	k := activeAgentKeyboard.Load()
	if k == nil || scope == 0 || uint64(scope) != k.scope.Load() {
		return
	}
	visible, _, _ := pWindowVisible.Call(k.window)
	listener, ok := cb.(interface{ OnAgentNavigate(int) })
	if visible == 0 || !ok || k.paused.Load() {
		return
	}
	if vk == 0x25 {
		listener.OnAgentNavigate(-1)
	} else if vk == 0x27 {
		listener.OnAgentNavigate(1)
	}
}
