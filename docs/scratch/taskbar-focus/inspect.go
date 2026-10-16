//go:build ignore

// Read-only taskbar geometry diagnostic; never linked into Agent Quota.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type rect struct{ Left, Top, Right, Bottom int32 }
type appbarData struct {
	Size           uint32
	Window         uintptr
	Callback, Edge uint32
	Rect           rect
	Param          uintptr
}
type node struct {
	Class    string
	Rect     rect
	Visible  bool
	Children []node `json:",omitempty"`
}

var (
	user             = windows.NewLazySystemDLL("user32.dll")
	find             = user.NewProc("FindWindowW")
	getWindow        = user.NewProc("GetWindow")
	getRect          = user.NewProc("GetWindowRect")
	getClass         = user.NewProc("GetClassNameW")
	visible          = user.NewProc("IsWindowVisible")
	systemParameters = user.NewProc("SystemParametersInfoW")
	appbar           = windows.NewLazySystemDLL("shell32.dll").NewProc("SHAppBarMessage")
)

func inspect(h uintptr, depth int) node {
	var name [256]uint16
	getClass.Call(h, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	n := node{Class: windows.UTF16ToString(name[:])}
	getRect.Call(h, uintptr(unsafe.Pointer(&n.Rect)))
	v, _, _ := visible.Call(h)
	n.Visible = v != 0
	if depth < 5 {
		child, _, _ := getWindow.Call(h, 5) // GW_CHILD
		for count := 0; child != 0 && count < 128; count++ {
			n.Children = append(n.Children, inspect(child, depth+1))
			child, _, _ = getWindow.Call(child, 2) // GW_HWNDNEXT
		}
	}
	return n
}

func main() {
	aware := user.NewProc("SetProcessDpiAwarenessContext")
	aware.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2
	class, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	h, _, _ := find.Call(uintptr(unsafe.Pointer(class)), 0)
	if h == 0 {
		fmt.Fprintln(os.Stderr, "primary taskbar unavailable")
		os.Exit(1)
	}
	var work rect
	if ok, _, err := systemParameters.Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0); ok == 0 {
		fmt.Fprintln(os.Stderr, "work area:", err)
		os.Exit(1)
	}
	data := appbarData{Window: h}
	data.Size = uint32(unsafe.Sizeof(data))
	if ok, _, err := appbar.Call(5, uintptr(unsafe.Pointer(&data))); ok == 0 { // ABM_GETTASKBARPOS
		fmt.Fprintln(os.Stderr, "taskbar position:", err)
		os.Exit(1)
	}
	state, _, _ := appbar.Call(4, uintptr(unsafe.Pointer(&data))) // ABM_GETSTATE
	result := struct {
		WorkArea rect
		Taskbar  rect
		Edge     uint32
		AutoHide bool
		Tree     node
	}{work, data.Rect, data.Edge, state&1 != 0, inspect(h, 0)}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
