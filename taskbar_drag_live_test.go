package main

import (
	"encoding/json"
	"image"
	"os"
	"sort"
	"testing"
	"time"
	"unsafe"

	"agent-quota/internal/win"
	"agent-quota/widget"
	"golang.org/x/sys/windows"
)

// This measures the production drag callbacks with a real layered window and
// Explorer connection, without moving the user's mouse or reading providers.
func checkLiveTaskbarDrag(t *testing.T, a *app, pump func(), path string) {
	t.Helper()
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	generation := uint64(win.TaskbarStatus()["session"].(uintptr))
	origin := a.taskbar.Snapshot().Readout.Min.Add(image.Pt(30, 20))
	a.OnTaskbarEvent(generation, 4, origin)
	if !a.nativeDrag {
		t.Fatal("native drag did not start")
	}
	a.OnTaskbarEvent(generation, 5, image.Pt(1000, 700))
	pump()
	measure := func(move func(int)) map[string]any {
		const samples = 120
		times := make([]float64, 0, samples)
		for i := 0; i < samples; i++ {
			start := time.Now()
			move(i)
			times = append(times, float64(time.Since(start))/float64(time.Millisecond))
		}
		sort.Float64s(times)
		total := 0.0
		for _, ms := range times {
			total += ms
		}
		return map[string]any{"samples": samples, "mean_ms": total / samples, "p95_ms": times[114], "max_ms": times[119]}
	}
	native := measure(func(i int) {
		p := image.Pt(1000+i%100, 700+i%100)
		a.OnTaskbarEvent(generation, 5, p)
		if image.Pt(a.x, a.y) != p.Sub(image.Pt(30, 20)) {
			t.Fatalf("native drag lost pointer anchor at %v: (%d,%d)", p, a.x, a.y)
		}
	})
	user := windows.NewLazySystemDLL("user32.dll")
	name, _ := windows.UTF16PtrFromString("AgentQuota.Taskbar.Input.1")
	input, _, _ := user.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(name)))
	owner := win.TaskbarStatus()["owner"].(uintptr)
	for i := 0; i < 120; i++ {
		p := image.Pt(1000+i%100, 700+i%100)
		packed := uint64(5) | uint64(p.X)<<16 | uint64(p.Y)<<40
		ok, _, _ := user.NewProc("PostMessageW").Call(owner, input, uintptr(generation), uintptr(packed))
		if ok == 0 {
			t.Fatal("cannot queue native movement")
		}
	}
	queuedAt := time.Now()
	pump()
	queuedMS := float64(time.Since(queuedAt)) / float64(time.Millisecond)
	if image.Pt(a.x, a.y) != image.Pt(989, 699) {
		t.Fatalf("queued movement did not reach the latest point: (%d,%d)", a.x, a.y)
	}
	a.OnTaskbarEvent(generation, 6, image.Pt(1019, 719))
	pump()
	a.OnDragStart()
	pill := widget.PillBounds(a.view)
	position := image.Pt(a.x-pill.Min.X, a.y-pill.Min.Y)
	floating := measure(func(i int) { a.OnMove(position.X+i%100, position.Y+i%100) })
	a.OnDragEnd()
	pump()
	record := map[string]any{"native_drag": native, "floating_drag": floating, "queued_120_moves_ms": queuedMS, "physical_pointer_injected": false}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	// A backlog must reach the current position promptly rather than replaying
	// every obsolete point. The deterministic queue test checks exact delivery;
	// this opt-in performance gate allows three 60 Hz frames for OS scheduling.
	if queuedMS >= 50 {
		t.Fatalf("queued taskbar movement takes %.2f ms to reach the current position", queuedMS)
	}
}
