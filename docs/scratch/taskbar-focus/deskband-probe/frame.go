//go:build ignore

// Frozen native-host fixtures only; this never invokes quota readers.
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"agent-quota/quota"
	"agent-quota/widget"
)

func writeFrame(dir, name string, src *image.RGBA, bounds image.Rectangle) error {
	img := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(img, img.Bounds(), src, bounds.Min, draw.Over)
	buf := make([]byte, 12+len(img.Pix))
	copy(buf, "AQF1")
	binary.LittleEndian.PutUint32(buf[4:], uint32(img.Bounds().Dx()))
	binary.LittleEndian.PutUint32(buf[8:], uint32(img.Bounds().Dy()))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(buf[12+i:12+i+4], []byte{img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255})
	}
	if err := os.WriteFile(filepath.Join(dir, name+".aqf"), buf, 0600); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	fmt.Printf("%s: %dx%d\n", name, bounds.Dx(), bounds.Dy())
	return closeErr
}

func main() {
	if len(os.Args) != 2 {
		panic("usage: go run frame.go OUTPUT_DIRECTORY")
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
	items := []widget.Item{
		{ID: quota.ItemClaude, State: quota.OK, Used: 19, HasPct: true, Windows: []quota.Window{{Label: "5h", Used: 19}, {Label: "week", Used: 70}}},
		{ID: quota.ItemKiro, State: quota.OK, Used: 3, HasPct: true},
		{ID: quota.ItemCodex, State: quota.OK, Used: 42, HasPct: true, Windows: []quota.Window{{Label: "5h", Used: 42}, {Label: "week", Used: 78}}, Reset: "5h resets in 1h 24m", PillReset: "1h24m", Details: []widget.Detail{{Label: "5h", Value: "42% used", Reset: "resets in 1h 24m"}, {Label: "Week", Value: "78% used", Reset: "resets in 3d 6h"}}},
		{ID: quota.ItemCursor, State: quota.OK, Used: 66, HasPct: true},
		{ID: quota.ItemCopilot, State: quota.OK, Used: 13, HasPct: true},
		{ID: quota.ItemDevin, State: quota.OK, Used: 38, HasPct: true},
		{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, HasPct: true},
	}
	for _, percent := range []int{100, 125, 150, 200} {
		v := widget.View{Items: items, Seam: true, Floating: true, Compact: true, Selected: quota.ItemCodex, Scale: float32(percent) / 100}
		img := widget.Render(v)
		if err := writeFrame(dir, fmt.Sprintf("readout%d", percent), img, widget.PillBounds(v)); err != nil {
			panic(err)
		}
		v.Open, v.Above, v.Checked = true, true, "DESKBAND PROBE · frozen fixture"
		img = widget.Render(v)
		if err := writeFrame(dir, fmt.Sprintf("popup%d", percent), img, img.Bounds()); err != nil {
			panic(err)
		}
	}
}
