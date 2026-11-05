package widget

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"agent-quota/internal/marks"
	"agent-quota/internal/raster"
	"agent-quota/internal/text"
	"agent-quota/quota"
)

type detailLine struct {
	value  string
	amount string
	reset  string
	used   float64
	meter  bool
	y      int
	style  text.Style
	ink    color.NRGBA
}

type detailLayout struct {
	lines                   []detailLine
	height                  int
	groups, reset, recovery image.Rectangle
}

func selectedDetailLayout(v View, width int) detailLayout {
	d := detailLayout{}
	it, ok := seamSelected(v)
	if !ok {
		return d
	}
	y := 10
	add := func(value string, style text.Style, ink color.NRGBA) {
		for _, paragraph := range strings.Split(value, "\n") {
			for _, line := range wrapDetail(paragraph, style, float32(width-48)) {
				d.lines = append(d.lines, detailLine{value: line, y: y, style: style, ink: ink})
				y += 15
			}
		}
	}
	title := quota.ItemShort(it.ID)
	if it.Plan != "" {
		title += " · " + it.Plan
	}
	add(title, titleFont, whiteInk)
	y += 4
	switch it.State {
	case quota.Waiting:
		add("Checking usage…", detailFont, dimInk)
	case quota.Failed:
		message := it.Err
		if message == "" {
			message = it.Detail
		}
		if v.RecoveryOpen == it.ID {
			message = it.Help
		}
		add(message, detailFont, errorInk)
	default:
		if len(it.Details) > 0 {
			cluster := metricCluster(it.Details)
			labelCol := float32(width-48) - cluster
			if labelCol < 64 {
				labelCol = 64
			}
			for _, row := range it.Details {
				amount, used, meter := metricAmount(row.Value)
				if text.Measure(row.Label, detailFont) <= labelCol {
					d.lines = append(d.lines, detailLine{
						value: row.Label, amount: amount, reset: row.Reset,
						used: used, meter: meter, y: y, style: detailFont, ink: whiteInk,
					})
					y += 16
				} else {
					for _, part := range wrapDetail(row.Label, detailFont, float32(width-48)) {
						add(part, detailFont, whiteInk)
					}
					d.lines = append(d.lines, detailLine{
						amount: amount, reset: row.Reset,
						used: used, meter: meter, y: y, style: detailFont, ink: whiteInk,
					})
					y += 16
				}
				y += 4
			}
		} else {
			add(seamDetail(it), detailFont, dimInk)
		}
		if it.Observed != "" {
			y += 2
			add("As of "+it.Observed, captionFont, mutedInk)
		}
		if it.State == quota.Stale {
			y += 4
			add("Last good "+it.Stale, detailFont, staleInk)
			add(it.Err, detailFont, errorInk)
		}
	}
	d.height = y + 8
	left := 24
	if len(it.Details) == 0 && len(it.Groups) > 0 && (it.State == quota.OK || it.State == quota.Stale) {
		d.groups = image.Rect(left, 6, left+112, 30)
		left += 120
	}
	if it.Reset != "" {
		d.reset = image.Rect(left, 6, left+84, 30)
	}
	if it.State == quota.Failed && it.Help != "" {
		d.recovery = image.Rect(left, 6, width-138, 30)
	}
	return d
}

func wrapDetail(value string, style text.Style, width float32) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(value) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if text.Measure(candidate, style) <= width {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
		for _, r := range word {
			candidate = line + string(r)
			if line != "" && text.Measure(candidate, style) > width {
				lines = append(lines, line)
				line = ""
			}
			line += string(r)
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func (p painter) drawSelectedDetails(v View) {
	d := selectedDetailLayout(v, p.g.w)
	stale := false
	if it, ok := seamSelected(v); ok {
		stale = it.State == quota.Stale
	}
	right := p.g.rect(p.g.w-24, 0, 0, 0).Min.X
	for _, line := range d.lines {
		at := p.g.rect(24, seamHeader+seamCells+line.y, 0, 0).Min
		p.paintDetailLine(p.c, at.X, at.Y, right, line, stale)
	}
}

func (p painter) paintDetailLine(dst *raster.Canvas, x, y, right int, line detailLine, stale bool) {
	labelW := float32(0)
	if line.value != "" {
		text.Draw(dst.Img, float32(x), float32(y), line.value, p.style(line.style), line.ink)
		labelW = text.Measure(line.value, p.style(line.style))
	}
	if line.amount == "" && line.reset == "" {
		return
	}
	resetW := float32(0)
	if line.reset != "" {
		resetW = text.Measure(line.reset, p.style(captionFont))
		text.Draw(dst.Img, float32(right)-resetW, float32(y), line.reset, p.style(captionFont), mutedInk)
	}
	if line.amount == "" {
		return
	}
	amountStyle := text.Style{Size: 11, Weight: 600}
	aw := text.Measure(line.amount, p.style(amountStyle))
	gap := p.step(8)
	barW := 0
	if line.meter {
		barW = p.step(36)
	}
	bx := x + int(math.Ceil(float64(labelW))) + gap
	ax := float32(bx + barW + gap)
	resetLeft := float32(right)
	if line.reset != "" {
		resetLeft = float32(right) - resetW - float32(p.step(12))
	}
	if room := resetLeft - ax; aw > room && room > 8 {
		line.amount = text.Elide(line.amount, p.style(amountStyle), room)
		aw = text.Measure(line.amount, p.style(amountStyle))
	}
	text.Draw(dst.Img, ax, float32(y), line.amount, p.style(amountStyle), whiteInk)
	if !line.meter {
		return
	}
	barH := max(p.step(3), 2)
	by := y + p.step(5)
	dst.FillRound(bx, by, barW, barH, max(barH/2, 1), color.RGBA{R: 255, G: 255, B: 255, A: 38})
	fill := int(math.Round(float64(barW) * min(max(line.used, 0), 100) / 100))
	if fill <= 0 {
		return
	}
	ink := marks.ForUsed(line.used)
	if stale {
		ink.A = 128
	}
	dst.FillRound(bx, by, fill, barH, max(barH/2, 1), ink)
}

func metricAmount(value string) (shown string, used float64, meter bool) {
	shown = strings.TrimSpace(value)
	if strings.HasSuffix(strings.ToLower(shown), " used") {
		shown = strings.TrimSpace(shown[:len(shown)-len(" used")])
	}
	if !strings.HasSuffix(shown, "%") {
		return shown, 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(shown, "%"), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return shown, 0, false
	}
	n = min(max(n, 0), 100)
	return strconv.Itoa(int(math.Round(n))) + "%", n, true
}

func metricCluster(rows []Detail) float32 {
	var amountW, resetW float32
	meter := false
	for _, row := range rows {
		amount, _, has := metricAmount(row.Value)
		if has {
			meter = true
		}
		if w := text.Measure(amount, text.Style{Size: 11, Weight: 600}); w > amountW {
			amountW = w
		}
		if w := text.Measure(row.Reset, captionFont); w > resetW {
			resetW = w
		}
	}
	cluster := amountW + resetW
	if resetW > 0 && amountW > 0 {
		cluster += 10
	}
	if meter {
		cluster += 36 + 8
	}
	return cluster
}
