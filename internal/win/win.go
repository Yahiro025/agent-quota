package win

import (
	"fmt"
	"image"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	pCreateWindowEx     = user32.NewProc("CreateWindowExW")
	pDefWindowProc      = user32.NewProc("DefWindowProcW")
	pRegisterClass      = user32.NewProc("RegisterClassExW")
	pShowWindow         = user32.NewProc("ShowWindow")
	pUpdateLayered      = user32.NewProc("UpdateLayeredWindow")
	pSetCapture         = user32.NewProc("SetCapture")
	pReleaseCapture     = user32.NewProc("ReleaseCapture")
	pGetMessage         = user32.NewProc("GetMessageW")
	pTranslateMessage   = user32.NewProc("TranslateMessage")
	pDispatchMessage    = user32.NewProc("DispatchMessageW")
	pPostQuitMessage    = user32.NewProc("PostQuitMessage")
	pPostMessage        = user32.NewProc("PostMessageW")
	pFindWindow         = user32.NewProc("FindWindowW")
	pSystemParameters   = user32.NewProc("SystemParametersInfoW")
	pSetTimer           = user32.NewProc("SetTimer")
	pKillTimer          = user32.NewProc("KillTimer")
	pGetCursorPos       = user32.NewProc("GetCursorPos")
	pTrackMouseEvent    = user32.NewProc("TrackMouseEvent")
	pCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	pAppendMenu         = user32.NewProc("AppendMenuW")
	pTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	pDestroyMenu        = user32.NewProc("DestroyMenu")
	pSetWindowPos       = user32.NewProc("SetWindowPos")
	pGetWindowRect      = user32.NewProc("GetWindowRect")
	pGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
	pSetDPIAware        = user32.NewProc("SetProcessDpiAwarenessContext")
	pGetDPIForWindow    = user32.NewProc("GetDpiForWindow")
	pGetDPIForSystem    = user32.NewProc("GetDpiForSystem")
	pLoadCursor         = user32.NewProc("LoadCursorW")
	pSetCursor          = user32.NewProc("SetCursor")
	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	pCreateMutex        = kernel32.NewProc("CreateMutexW")
	pFreeConsole        = kernel32.NewProc("FreeConsole")
)

const (
	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolwindow = 0x00000080
	wsExNoactivate = 0x08000000
	wsPopup        = 0x80000000

	swShowNoactivate = 4

	wmDestroy       = 0x0002
	wmMouseactivate = 0x0021
	wmSetcursor     = 0x0020
	maNoactivate    = 3
	wmMousemove     = 0x0200
	wmLbuttondown   = 0x0201
	wmLbuttonup     = 0x0202
	wmRbuttonup     = 0x0205
	wmMouseleave    = 0x02A3
	wmTimer         = 0x0113
	wmDpichanged    = 0x02E0
	wmCommand       = 0x0111

	ulwAlpha     = 2
	acSrcOver    = 0
	acSrcAlpha   = 1
	biRgb        = 0
	dibRgbColors = 0

	tmeLeave = 2

	mfString    = 0
	mfChecked   = 8
	mfSeparator = 0x800

	tpmRightbutton = 2
	tpmReturncmd   = 0x100

	swpNosize     = 1
	swpNomove     = 2
	swpNoactivate = 0x10
	swpShowwindow = 0x40
	hwndTopmost   = ^uintptr(0)
	smCxscreen    = 0
	smCyscreen    = 1

	// Timer IDs.
	tPoll  = 1
	tHover = 2
	tLeave = 3
)

// MenuItem is one right-click menu row.
type MenuItem struct {
	ID      int
	Label   string
	Checked bool
	Sep     bool
}

// Callback is the app's event sink.
type Callback interface {
	Paint(w, h int) []byte // BGRA, stride w*4
	OnMove(x, y int)
	OnClick()
	OnHover(open bool)
	OnMenu(id int)
	Menu() []MenuItem
	Size() (w, h int)
}

var cb Callback
var hwnd windows.HWND
var singleMutex windows.Handle

var (
	dragging       bool
	downSX, downSY int
	downWX, downWY int
	moved          bool
)

type point struct {
	x, y int32
}

func cursorPos() (int, int) {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.x), int(p.y)
}

func windowRect() (x, y, w, h int) {
	var r [4]int32
	pGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&r[0])))
	return int(r[0]), int(r[1]), int(r[2] - r[0]), int(r[3] - r[1])
}

// HoldSingle claims the Local\AgentQuotaRunning mutex for the life of the
// process. It reports false when another copy already holds it (caller exits).
// The handle stays open; closing it would release the claim.
// NOTE: windows.GetLastError reads nil here; the errno from Call is the signal.
func HoldSingle() bool {
	name, _ := windows.UTF16PtrFromString(`Local\AgentQuotaRunning`)
	h, _, errno := pCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	if errno == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(windows.Handle(h))
		return false
	}
	singleMutex = windows.Handle(h)
	runtime.KeepAlive(name)
	return true
}

// FreeConsole detaches the process from its launching console so window
// mode never blocks a terminal. --print and --shot never call it.
func FreeConsole() { pFreeConsole.Call() }

// SecondInstance reports whether the mutex already existed.
func SecondInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\AgentQuotaRunning`)
	h, _, errno := pCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	exists := errno == windows.ERROR_ALREADY_EXISTS
	windows.CloseHandle(windows.Handle(h))
	return exists
}

type wndClassEx struct {
	size, style        uint32
	proc               uintptr
	clsExtra, wndExtra int32
	inst               windows.Handle
	icon, cursor, bg   windows.Handle
	menu, className    *uint16
	iconSm             windows.Handle
}

type trackMouse struct {
	size, flags uint32
	hwnd        windows.HWND
	hoverTime   uint32
}

func armLeave() {
	var t trackMouse
	t.size = uint32(unsafe.Sizeof(t))
	t.flags = tmeLeave
	t.hwnd = hwnd
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
}

func showMenu() {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	for _, it := range cb.Menu() {
		if it.Sep {
			pAppendMenu.Call(m, mfSeparator, 0, 0)
			continue
		}
		s, _ := windows.UTF16PtrFromString(it.Label)
		flags := uintptr(mfString)
		if it.Checked {
			flags |= mfChecked
		}
		pAppendMenu.Call(m, flags, uintptr(it.ID), uintptr(unsafe.Pointer(s)))
		runtime.KeepAlive(s)
	}
	sx, sy := cursorPos()
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmRightbutton|tpmReturncmd, uintptr(sx), uintptr(sy), 0, uintptr(hwnd), 0)
	if cmd != 0 {
		cb.OnMenu(int(cmd))
	}
}

func wndProc(h windows.HWND, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case wmSetcursor:
		cursor := uintptr(32512)
		if pointer, ok := cb.(interface{ PointerHand() bool }); ok && pointer.PointerHand() {
			cursor = 32649
		}
		handle, _, _ := pLoadCursor.Call(0, cursor)
		pSetCursor.Call(handle)
		return 1
	case wmMouseactivate:
		return maNoactivate
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	case wmMousemove:
		armLeave()
		pKillTimer.Call(uintptr(h), tLeave)
		if pointer, ok := cb.(interface{ OnPointerMove(int, int) }); ok {
			pointer.OnPointerMove(int(int16(l&0xFFFF)), int(int16(l>>16)))
		}
		// Hover: open the panel after ~400 ms.
		pSetTimer.Call(uintptr(h), tHover, 400, 0)
		if dragging {
			sx, sy := cursorPos()
			if abs(sx-downSX)+abs(sy-downSY) > 4 {
				moved = true
			}
			if moved {
				cb.OnMove(downWX+sx-downSX, downWY+sy-downSY)
			}
		}
		return 0
	case wmLbuttondown:
		sx, sy := cursorPos()
		wx, wy, _, _ := windowRect()
		downSX, downSY = sx, sy
		downWX, downWY = wx, wy
		dragging, moved = true, false
		pSetCapture.Call(uintptr(h))
		return 0
	case wmLbuttonup:
		if dragging {
			dragging = false
			pReleaseCapture.Call()
			if !moved {
				if pointer, ok := cb.(interface{ OnPointerClick(int, int) }); ok {
					pointer.OnPointerClick(int(int16(l&0xFFFF)), int(int16(l>>16)))
				} else {
					cb.OnClick()
				}
			} else if end, ok := cb.(interface{ OnDragEnd() }); ok {
				end.OnDragEnd()
			}
		}
		return 0
	case wmRbuttonup:
		showMenu()
		return 0
	case wmMouseleave:
		if pointer, ok := cb.(interface{ OnPointerMove(int, int) }); ok {
			pointer.OnPointerMove(-1, -1)
		}
		pKillTimer.Call(uintptr(h), tHover)
		// Leaving closes the panel after ~300 ms unless pinned.
		pSetTimer.Call(uintptr(h), tLeave, 300, 0)
		return 0
	case wmTimer:
		switch w {
		case tHover:
			pKillTimer.Call(uintptr(h), tHover)
			cb.OnHover(true)
		case tLeave:
			pKillTimer.Call(uintptr(h), tLeave)
			cb.OnHover(false)
		default:
			// Poll and DPI timers belong to the app.
			cb.OnMenu(int(w))
		}
		return 0
	case wmCommand:
		cb.OnMenu(int(w & 0xFFFF))
		return 0
	case wmDpichanged:
		if dpi, ok := cb.(interface{ OnDPIChanged(float32) }); ok {
			dpi.OnDPIChanged(float32(w&0xFFFF) / 96)
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(h), uintptr(msg), w, l)
	return r
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Create makes the layered NOACTIVATE popup at (x,y).
func Create(title string, x, y int, c Callback) error {
	cb = c
	cls, _ := windows.UTF16PtrFromString("AgentQuota")
	t, _ := windows.UTF16PtrFromString(title)
	proc := windows.NewCallback(wndProc)
	var kc wndClassEx
	kc.size = uint32(unsafe.Sizeof(kc))
	kc.proc = proc
	kc.className = cls
	if r, _, _ := pRegisterClass.Call(uintptr(unsafe.Pointer(&kc))); r == 0 {
		// Already registered from a prior run in this process (tests).
	}
	w, h := c.Size()
	r, _, _ := pCreateWindowEx.Call(
		uintptr(wsExLayered|wsExTopmost|wsExToolwindow|wsExNoactivate),
		uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(t)),
		uintptr(wsPopup), uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		0, 0, 0, 0)
	hwnd = windows.HWND(r)
	if hwnd == 0 {
		return windows.GetLastError()
	}
	pShowWindow.Call(uintptr(hwnd), swShowNoactivate)
	runtime.KeepAlive(proc)
	return nil
}

// Resize moves/resizes the popup without activating it.
func Resize(x, y, w, h int) {
	if hwnd == 0 {
		return
	}
	pSetWindowPos.Call(uintptr(hwnd), hwndTopmost, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoactivate|swpShowwindow)
}

// StartPoll arms the 30 s refresh timer.
func StartPoll() {
	if hwnd == 0 {
		return
	}
	pSetTimer.Call(uintptr(hwnd), tPoll+100, 30_000, 0)
}

// ScreenSize is the primary monitor size in pixels.
func ScreenSize() (w, h int) {
	w32, _, _ := pGetSystemMetrics.Call(smCxscreen)
	h32, _, _ := pGetSystemMetrics.Call(smCyscreen)
	return int(w32), int(h32)
}

// WorkArea excludes the primary monitor's taskbar and docked desktop bars.
func WorkArea() image.Rectangle {
	var rect [4]int32
	if r, _, _ := pSystemParameters.Call(0x30, 0, uintptr(unsafe.Pointer(&rect)), 0); r != 0 {
		return image.Rect(int(rect[0]), int(rect[1]), int(rect[2]), int(rect[3]))
	}
	w, h := ScreenSize()
	return image.Rect(0, 0, w, h)
}

// NotifyExisting lets a second launch ask the current window to reveal itself.
// Posting preserves the owner's UI thread and never activates its window.
func NotifyExisting(title string, id int) bool {
	cls, _ := windows.UTF16PtrFromString("AgentQuota")
	name, _ := windows.UTF16PtrFromString(title)
	h, _, _ := pFindWindow.Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	r, _, _ := pPostMessage.Call(h, wmCommand, uintptr(id), 0)
	return r != 0
}

// Draw at the monitor's physical resolution instead of letting Windows stretch
// a 96-DPI bitmap, which makes small Inter text and logo tiles look blurred.
func EnableDPIAwareness() { pSetDPIAware.Call(^uintptr(3)) }

func DPIScale() float32 {
	var dpi uintptr
	if hwnd != 0 {
		dpi, _, _ = pGetDPIForWindow.Call(uintptr(hwnd))
	} else {
		dpi, _, _ = pGetDPIForSystem.Call()
	}
	if dpi == 0 {
		return 1
	}
	return float32(dpi) / 96
}

// Quit posts the quit message.
func Quit() { pPostQuitMessage.Call(0) }

// PostMenu queues an app notification from any goroutine. Using PostMessage
// avoids waiting on the owner thread, which may be handling a mouse event.
func PostMenu(id int) {
	pPostMessage.Call(uintptr(hwnd), wmCommand, uintptr(id), 0)
}

// Flush pushes BGRA pixels (stride w*4) via UpdateLayeredWindow.
func Flush(bgra []byte, w, h int) error {
	type bmpInfo struct {
		size, width, height         uint32
		planes, bpp                 uint16
		comp, imgSize               uint32
		xppm, yppm, clrUsed, clrImp uint32
	}
	var info bmpInfo
	info.size = 40
	info.width = uint32(w)
	info.height = uint32(-h) // top-down
	info.planes = 1
	info.bpp = 32
	info.comp = biRgb
	dc, _, _ := pCreateCompatibleDC.Call(0)
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&info)), dibRgbColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	old, _, _ := pSelectObject.Call(dc, bmp)
	dst := unsafe.Slice((*byte)(bits), len(bgra))
	copy(dst, bgra)
	// Win32 takes a pointer to four BYTEs, with full global opacity so the
	// bitmap's per-pixel alpha controls transparency.
	blend := [4]byte{acSrcOver, 0, 255, acSrcAlpha}
	size := point{int32(w), int32(h)}
	var src point
	r, _, err := pUpdateLayered.Call(uintptr(hwnd), 0, 0, uintptr(unsafe.Pointer(&size)), dc, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	pSelectObject.Call(dc, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(dc)
	if r == 0 {
		return fmt.Errorf("UpdateLayeredWindow: %w", err)
	}
	return nil
}

// Run pumps messages until quit.
func Run() {
	var msg [8]uintptr
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0)
		if r == 0 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
}

// Loop initializes the window and pumps its messages until quit.
func Loop(start func() error) error {
	// Win32 sends input to the thread that created the HWND. Go must not
	// move this goroutine to another thread between creation and dispatch.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := start(); err != nil {
		return err
	}
	Run()
	return nil
}
