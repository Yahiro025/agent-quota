package win

import (
	"encoding/binary"
	"image"
	"runtime"
	"slices"
	"testing"
	"unsafe"
)

type taskbarEvent struct {
	generation uint64
	kind       int
	point      image.Point
}

type taskbarInputCallback struct {
	testCallback
	events []taskbarEvent
}

func (c *taskbarInputCallback) OnTaskbarEvent(generation uint64, kind int, p image.Point) {
	c.events = append(c.events, taskbarEvent{generation, kind, p})
}

func TestTaskbarDragDropsBacklogWithoutCrossingInputBoundaries(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	callback := &taskbarInputCallback{}
	if err := Create("AgentQuota drag queue regression", 100, 100, callback); err != nil {
		t.Fatal(err)
	}
	peek := user32.NewProc("PeekMessageW")
	defer func() {
		user32.NewProc("DestroyWindow").Call(uintptr(hwnd))
		hwnd = 0
		var msg [8]uintptr
		peek.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0x12, 0x12, 1)
	}()
	post := func(generation uint64, kind, x, y int) {
		t.Helper()
		packed := uint64(kind) | (uint64(uint32(x)&0xffffff) << 16) | (uint64(uint32(y)&0xffffff) << 40)
		ok, _, _ := pPostMessage.Call(uintptr(hwnd), uintptr(taskbarInput), uintptr(generation), uintptr(packed))
		if ok == 0 {
			t.Fatal("cannot queue taskbar input")
		}
	}
	post(9, 4, 10, 20)
	for i := 0; i < 120; i++ {
		post(9, 5, 100+i, 200+i)
	}
	post(9, 1, 0, 0) // A geometry event must keep its place between move runs.
	post(9, 5, 300, 400)
	post(9, 5, 301, 401)
	post(9, 6, 302, 402) // Release and its final point must survive.
	post(10, 5, -100, -200)
	post(11, 5, -101, -201) // Do not merge different connection generations.
	post(11, 7, 0, 0)       // Capture cancellation is another ordering boundary.
	post(11, 5, -102, -202)
	for {
		var msg [8]uintptr
		ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg[0])), uintptr(hwnd), uintptr(taskbarInput), uintptr(taskbarInput), 1)
		if ok == 0 {
			break
		}
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
	}
	want := []taskbarEvent{
		{9, 4, image.Pt(10, 20)}, {9, 5, image.Pt(219, 319)},
		{9, 1, image.Point{}}, {9, 5, image.Pt(301, 401)}, {9, 6, image.Pt(302, 402)},
		{10, 5, image.Pt(-100, -200)}, {11, 5, image.Pt(-101, -201)},
		{11, 7, image.Point{}}, {11, 5, image.Pt(-102, -202)},
	}
	if !slices.Equal(callback.events, want) {
		t.Fatalf("drag delivered %d events, want %d latest-position/control events; got %v", len(callback.events), len(want), callback.events)
	}
}

func TestTaskbarPacket(t *testing.T) {
	pixels := make([]byte, 148*48*4)
	packet, err := taskbarPacket(9, 5, 148, 48, 120, pixels)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet) != 32+len(pixels) || binary.LittleEndian.Uint64(packet[8:]) != 9 || binary.LittleEndian.Uint32(packet[28:]) != taskbarProtocol {
		t.Fatal("wire header changed")
	}
	for _, bad := range []struct {
		w, h, d int
		flags   uint32
		gen     uint64
	}{{1025, 48, 120, 1, 9}, {148, 257, 120, 1, 9}, {148, 48, 95, 1, 9}, {148, 48, 120, 8, 9}, {148, 48, 120, 1, 0}} {
		if _, err := taskbarPacket(bad.gen, bad.flags, bad.w, bad.h, bad.d, pixels); err == nil {
			t.Fatalf("accepted invalid frame: %+v", bad)
		}
	}
	if _, err := taskbarPacket(9, 2, 0, 0, 96, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTaskbarInputSignedCoordinates(t *testing.T) {
	for _, point := range []image.Point{image.Pt(-1920, -1200), image.Pt(1919, 1199), image.Pt(0, 0)} {
		packed := uint64(5) | (uint64(uint32(point.X)&0xffffff) << 16) | (uint64(uint32(point.Y)&0xffffff) << 40)
		kind, p := decodeTaskbarInput(uintptr(packed))
		if kind != 5 || p != point {
			t.Fatalf("got %d %v want %v", kind, p, point)
		}
	}
}

func TestTaskbarDockTarget(t *testing.T) {
	s := TaskbarSnapshot{Available: true, Bounds: image.Rect(0, 1150, 1920, 1200), Tray: image.Rect(1545, 1150, 1920, 1200), DPI: 120}
	if !s.DockTarget(image.Pt(1500, 1175), 148) || s.DockTarget(image.Pt(1500, 1149), 148) || s.DockTarget(image.Pt(1800, 1175), 148) {
		t.Fatal("snap target must stay in taskbar before tray")
	}
	s.Available = false
	if s.DockTarget(image.Pt(1500, 1175), 148) {
		t.Fatal("unavailable native host must not offer docking")
	}
}
