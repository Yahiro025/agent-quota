package main

import (
	"image"
	"image/draw"
	"math"

	"agent-quota/internal/core"
	"agent-quota/internal/win"
	"agent-quota/widget"
)

func bgraPixels(img *image.RGBA) []byte {
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			out = append(out, img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3])
		}
	}
	return out
}

func focusPopupPosition(readout, work image.Rectangle, w, h int) image.Point {
	x := readout.Min.X + (readout.Dx()-w)/2
	y := min(work.Max.Y, readout.Min.Y) - h - 8
	x, y = clampWorkPos(x, y, w, h, work)
	return image.Pt(x, y)
}

func (a *app) syncTaskbar() {
	if a.taskbar == nil {
		return
	}
	snapshot := a.taskbar.Snapshot()
	if !snapshot.Available {
		a.docked = false
		return
	}
	v := a.buildView()
	v.Focus = true
	v.Open = false
	v.Scale = float32(snapshot.DPI) / 96
	img := widget.RenderTaskbar(v)
	bounds := img.Bounds()
	visible := (a.settings.Docked || a.nativeDrag) && bounds.Dy()+2 <= snapshot.Bounds.Dy()
	if !a.taskbar.Frame(bgraPixels(img), bounds.Dx(), bounds.Dy(), snapshot.DPI, visible) {
		a.docked = false
		return
	}
	snapshot = a.taskbar.Snapshot()
	a.docked = a.settings.Docked && !a.nativeDrag && snapshot.Available && snapshot.Fits
	if a.docked {
		a.scale = float32(snapshot.DPI) / 96
	}
}

func (a *app) drawTaskbarPopup(v widget.View) bool {
	if !a.docked {
		return false
	}
	snapshot := a.taskbar.Snapshot()
	if !a.open || snapshot.Hidden {
		win.SetVisible(false)
		return true
	}
	img := widget.Render(v)
	bounds := widget.FocusPopupBounds(v)
	popup := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(popup, popup.Bounds(), img, bounds.Min, draw.Src)
	at := focusPopupPosition(snapshot.Readout, win.WorkArea(), bounds.Dx(), bounds.Dy())
	a.w, a.h = bounds.Dx(), bounds.Dy()
	win.Resize(at.X, at.Y, a.w, a.h)
	if err := win.Flush(bgraPixels(popup), a.w, a.h); err != nil {
		core.Logf("taskbar popup: %v", err)
	}
	return true
}

func (a *app) updateDockPreview(pointer image.Point) {
	a.dockPreview = false
	if a.taskbar != nil {
		s := a.taskbar.Snapshot()
		a.dockPreview = s.DockTarget(pointer, int(math.Round(118*float64(s.DPI)/96)))
	}
}

func (a *app) finishDockDrag() {
	a.settings.Docked = a.dockPreview
	if a.settings.Docked {
		a.settings.X, a.settings.Y = a.floatAnchor.X, a.floatAnchor.Y
		a.x, a.y = a.floatAnchor.X, a.floatAnchor.Y
	}
	a.nativeDrag, a.windowDrag, a.dockPreview = false, false, false
	a.open, a.pinned = false, false
	a.redraw()
	saveSettings(a.settings)
}

func (a *app) OnTaskbarEvent(generation uint64, kind int, p image.Point) {
	if a.taskbar == nil || (kind != 0 && !a.taskbar.Owns(generation)) {
		return
	}
	switch kind {
	case 0, 1:
		a.redraw()
	case 2:
		if a.docked {
			a.OnClick()
		}
	case 4:
		if !a.docked {
			return
		}
		s := a.taskbar.Snapshot()
		a.nativeOffset = p.Sub(s.Readout.Min)
		a.floatAnchor = image.Pt(a.settings.X, a.settings.Y)
		a.nativeDrag = true
		a.docked = false
		a.open = false
		a.pinned = false
		a.x, a.y = p.X-a.nativeOffset.X, p.Y-a.nativeOffset.Y
		a.redraw()
	case 5:
		if !a.nativeDrag {
			return
		}
		a.updateDockPreview(p)
		a.x, a.y = p.X-a.nativeOffset.X, p.Y-a.nativeOffset.Y
		a.redraw()
	case 6:
		if a.nativeDrag {
			a.updateDockPreview(p)
			a.finishDockDrag()
		}
	case 7:
		if a.nativeDrag {
			a.nativeDrag = false
			a.dockPreview = false
			a.x, a.y = a.floatAnchor.X, a.floatAnchor.Y
			a.settings.X, a.settings.Y = a.x, a.y
			a.redraw()
		}
	case 8:
		win.ShowMenu()
	case 9:
		a.docked = false
		a.nativeDrag = false
		a.dockPreview = false
		a.x, a.y = a.settings.X, a.settings.Y
		a.redraw()
	}
}

func (a *app) OnScroll(delta int) {
	if !a.docked || !a.open {
		return
	}
	a.focusScroll = max(0, min(widget.FocusScrollLimit(a.view), a.focusScroll-delta/4))
	a.redraw()
}
