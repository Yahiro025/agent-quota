package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agent-quota/internal/core"
	"agent-quota/internal/proc"
	"agent-quota/internal/win"
	"agent-quota/quota"
	"agent-quota/widget"
)

type settings struct {
	X, Y    int
	HasPos  bool
	Visible map[string]bool
}

func loadSettings() settings {
	s := settings{Visible: map[string]bool{}}
	for _, id := range quota.ItemAll {
		s.Visible[id] = true
	}
	b, err := os.ReadFile(core.SettingsFile())
	if err != nil {
		return s
	}
	var raw struct {
		X, Y    *int
		Visible map[string]bool
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return s
	}
	if raw.X != nil && raw.Y != nil {
		s.X, s.Y = *raw.X, *raw.Y
		s.HasPos = true
	}
	for k, v := range raw.Visible {
		s.Visible[k] = v
	}
	return s
}

func saveSettings(s settings) {
	raw := map[string]any{"x": s.X, "y": s.Y, "visible": s.Visible}
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(core.SettingsFile()), 0o755)
	tmp := core.SettingsFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, core.SettingsFile())
}

func checkedAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "Checked just now"
	case d < 90*time.Minute:
		return fmt.Sprintf("Checked %d min ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("Checked %d h ago", int(d.Hours()))
	}
}

// resetLabels formats one 5h reset instant into the panel detail line and the
// pill's second line, so both surfaces count down the same instant and can
// never disagree the way a parsed Detail tail could. clock picks the
// wall-clock form; otherwise both countdown, floored to whole minutes so the
// 30 s repaint never flickers on seconds. A zero or already passed reset —
// the reading can age past it between poll and draw — yields empty strings so
// the row simply drops its reset line.
func resetLabels(r time.Time, now time.Time, clock bool) (panel, pill string) {
	if r.IsZero() || !r.After(now) {
		return "", ""
	}
	if !clock {
		mins := int(r.Sub(now).Minutes())
		if h, m := mins/60, mins%60; h > 0 {
			return fmt.Sprintf("5h resets in %dh %dm", h, m), fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("5h resets in %dm", mins), fmt.Sprintf("%dm", mins)
	}
	pill = quota.FormatLocal(r, "HH:mm")
	// The panel has room to disambiguate a reset that lands on another day;
	// the pill doesn't, so it always stays plain HH:mm.
	rl, nl := r.In(time.Local), now.In(time.Local)
	if rl.Year() != nl.Year() || rl.YearDay() != nl.YearDay() {
		return "5h resets " + quota.FormatLocal(r, "d MMM HH:mm"), pill
	}
	return "5h resets " + pill, pill
}

func printOnce() int {
	failed := false
	for _, id := range quota.ItemAll {
		r := quota.ByID(id)
		used := "—"
		if r.OK() {
			used = fmt.Sprintf("%s%%", quota.Custom(*r.Used, 0))
		} else {
			failed = true
		}
		fmt.Printf("%-8s %-5s %s\n", id, used, r.Detail)
	}
	if failed {
		return 1
	}
	return 0
}

func shot(dir string) int {
	_ = os.MkdirAll(dir, 0o755)
	states := map[string]widget.View{
		"pill": {Items: []widget.Item{
			// Static Reset/PillReset pairs (countdown on claude/codex, clock on
			// antigravity) exercise both toggle forms without a clock, so the
			// shot stays byte-stable between runs.
			{ID: "claude", State: quota.OK, Used: 42, Detail: "Pro · 5h 42% · week 31%", Reset: "5h resets in 2h 14m", PillReset: "2h14m"},
			{ID: "kiro", State: quota.OK, Used: 75, Detail: "KIRO PRO · 21 of 50 credits"},
			{ID: "codex", State: quota.OK, Used: 95, Detail: "5h 95% · week 40%", Reset: "5h resets in 38m", PillReset: "38m"},
			{ID: "cursor", State: quota.Failed, Err: "Sign in to Cursor first."},
			{ID: "copilot", State: quota.OK, Used: 13, Detail: "Individual · premium 26/200"},
			{ID: "devin", State: quota.Stale, Used: 62, Err: "Couldn't reach Devin's server.", Stale: "3 min"},
			{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Detail: "Gemini Models · 90% remaining", Reset: "5h resets 18:42", PillReset: "18:42"},
		}, Checked: "Checked 2 min ago"},
	}
	// panel and empty are built only here: literals in the map above would be
	// dead code the two assignments overwrite anyway.
	panel := states["pill"]
	panel.Open = true
	states["panel"] = panel
	states["empty"] = widget.View{Open: true}
	panel.Scale = 1.5
	states["panel-150"] = panel
	pill := states["pill"]
	pill.Scale = 1.5
	states["pill-150"] = pill
	for name, v := range states {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		_ = png.Encode(f, widget.Render(v))
		f.Close()
	}
	return 0
}

func main() {
	printFlag := flag.Bool("print", false, "read each agent once and print results")
	shotDir := flag.String("shot", "", "write pill/panel PNGs to DIR")
	statuslineFlag := flag.Bool("claude-statusline", false, "receive Claude Code statusLine input")
	installClaudeFeedFlag := flag.Bool("install-claude-feed", false, "connect Claude Code statusLine to Agent Quota")
	flag.Parse()
	if *statuslineFlag {
		handleClaudeStatusline()
		return
	}
	if *installClaudeFeedFlag {
		exe, err := os.Executable()
		if err == nil {
			err = installClaudeFeedAt(quota.ClaudeHome(), core.Support(), exe)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not connect Claude Code to Agent Quota.")
			os.Exit(1)
		}
		return
	}
	if *printFlag {
		os.Exit(printOnce())
	}
	if *shotDir != "" {
		os.Exit(shot(*shotDir))
	}
	// Window mode never blocks the launching terminal: re-enter detached and
	// let this copy exit at once. --print and --shot return earlier instead.
	if os.Getenv("AGENTQUOTA_DETACHED") == "" {
		detachSelf()
	}
	if !win.HoldSingle() {
		// The mutex can be acquired just before the first copy creates its
		// window. Allow that short startup gap without spawning another one.
		deadline := time.Now().Add(time.Second)
		for !win.NotifyExisting("AgentQuota", menuReveal) {
			if time.Now().After(deadline) {
				core.Logf("couldn't reveal the running widget")
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		return
	}
	runWindow()
}

// detachSelf re-runs this binary without a console and exits this copy.
// proc.Hidden sets CREATE_NO_WINDOW, which also detaches the child.
func detachSelf() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := proc.Hidden(exe)
	cmd.Env = append(os.Environ(), "AGENTQUOTA_DETACHED=1")
	if err := cmd.Start(); err != nil {
		return
	}
	os.Exit(0)
}

// Menu IDs.

// Menu IDs.
const (
	menuRefresh   = 1
	menuAutostart = 2
	menuQuit      = 3
	menuChanged   = 4
	menuReveal    = 5
	menuAgentBase = 10
)

type app struct {
	visibilityMu   sync.RWMutex
	settings       settings
	poller         *quota.Poller
	open           bool
	pinned         bool
	checkedAt      time.Time
	w, h           int
	x, y           int
	scale          float32
	refreshHovered bool
	view           widget.View
	// resetClock flips a reset row between countdown and wall clock for that
	// agent's panel and pill strings. In-memory only (never in settings.json):
	// a fresh launch shows countdowns again. UI thread only, like open/pinned,
	// so no mutex.
	resetClock map[string]bool
}

func runWindow() {
	win.EnableDPIAwareness()
	s := loadSettings()
	a := &app{settings: s, checkedAt: time.Now(), scale: win.DPIScale(), resetClock: map[string]bool{}}
	a.poller = quota.SystemPoller(a.isOn, a.changed)
	// First launch lands top-right; otherwise clamp the saved spot on screen.
	pw, ph := widget.Size(widget.View{Items: make([]widget.Item, len(visibleAgents(s))), Scale: a.scale})
	s.X, s.Y = initialPosition(s, pw, ph, a.scale, win.WorkArea())
	s.HasPos = true
	a.x, a.y, a.w, a.h = s.X, s.Y, pw, ph
	a.settings = s
	if err := win.Loop(func() error {
		if err := win.Create("AgentQuota", a.x, a.y, a); err != nil {
			return err
		}
		a.scale = win.DPIScale()
		a.redraw()
		a.poller.Refresh(false)
		win.StartPoll()
		return nil
	}); err != nil {
		core.Logf("window: %v", err)
		os.Exit(1)
	}
	saveSettings(a.settings)
}

// Read completions check visibility off-thread while the menu can change it.
func (a *app) isOn(id string) bool {
	a.visibilityMu.RLock()
	defer a.visibilityMu.RUnlock()
	return a.settings.Visible[id]
}

func visibleAgents(s settings) []string {
	var out []string
	for _, id := range quota.ItemAll {
		if !s.Visible[id] || !quota.Present(id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func clampWorkPos(x, y, w, h int, work image.Rectangle) (int, int) {
	return max(work.Min.X, min(x, work.Max.X-w)), max(work.Min.Y, min(y, work.Max.Y-h))
}

// Recover an unusable saved position to the same discoverable corner used on
// first launch. A valid user-selected position is preserved.
func initialPosition(s settings, w, h int, scale float32, work image.Rectangle) (int, int) {
	if s.HasPos && image.Rect(s.X, s.Y, s.X+w, s.Y+h).In(work) {
		return s.X, s.Y
	}
	inset := int(math.Round(float64(24 * scale)))
	return clampWorkPos(work.Max.X-w-inset, work.Min.Y+inset, w, h, work)
}

func (a *app) buildView() widget.View {
	ids := visibleAgents(a.settings)
	// The single clock read for every reset countdown; widget stays pure.
	now := time.Now()
	items := make([]widget.Item, 0, len(ids))
	for _, id := range ids {
		st, show, err, ok := a.poller.Display(id)
		it := widget.Item{ID: id}
		if !ok {
			it.State = quota.Waiting
			it.Detail = "Checking…"
			items = append(items, it)
			continue
		}
		it.State = st
		it.Err = err
		switch st {
		case quota.OK, quota.Stale:
			if show.Used != nil {
				it.Used = *show.Used
			}
			it.Detail = show.Detail
			if st == quota.Stale {
				if g, at, ok := a.poller.LastGoodAt(id); ok {
					_ = g
					it.Stale = agoShort(at)
				} else {
					it.Stale = "a while"
				}
			}
		case quota.Failed:
			it.Detail = err
		default:
			it.Detail = "Checking…"
		}
		// Only OK rows carry a 5h-window reset: Stale keeps its error line
		// uncluttered and Failed/Waiting have none to toggle. Reading.Reset5h
		// is the instant; resetLabels formats both already-toggled strings
		// from it. Detail's trailing " · resets X" tail repeats that same
		// instant, so it is stripped from this row's copy when X is Reset5h
		// formatted either way the readers write it — a tail naming a
		// different instant (the week window's reset) stays visible so it is
		// not lost. show.Detail upstream is never mutated, and without a
		// usable Reset5h Detail stays byte-identical.
		if st == quota.OK && !show.Reset5h.IsZero() {
			it.Reset, it.PillReset = resetLabels(show.Reset5h, now, a.resetClock[id])
			if j := strings.LastIndex(it.Detail, " · resets "); j >= 0 {
				tail := strings.TrimSpace(it.Detail[j+len(" · resets "):])
				if tail == quota.FormatLocal(show.Reset5h, "HH:mm") || tail == quota.FormatLocal(show.Reset5h, "d MMM HH:mm") {
					it.Detail = it.Detail[:j]
				}
			}
		}
		items = append(items, it)
	}
	v := widget.View{Items: items, Open: a.open, Checked: checkedAgo(a.checkedAt), Scale: a.scale, RefreshHovered: a.refreshHovered}
	work := win.WorkArea()
	_, h := widget.Size(v)
	v.Above = a.open && a.y+h > work.Max.Y
	return v
}

func agoShort(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < 90*time.Minute {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%d h ago", int(d.Hours()))
}

func (a *app) redraw() {
	v := a.buildView()
	a.view = v
	img := widget.Render(v)
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	bgra := make([]byte, 0, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(x, y)
			bgra = append(bgra, img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3])
		}
	}
	a.w, a.h = w, h
	pill := widget.PillBounds(v)
	x, y := clampWorkPos(a.x-pill.Min.X, a.y-pill.Min.Y, w, h, win.WorkArea())
	a.x, a.y = x+pill.Min.X, y+pill.Min.Y
	a.settings.X, a.settings.Y = a.x, a.y
	a.settings.HasPos = true
	win.Resize(x, y, w, h)
	if err := win.Flush(bgra, w, h); err != nil {
		core.Logf("paint: %v", err)
	}
}

func (a *app) changed() {
	win.PostMenu(menuChanged)
}

// Size implements win.Callback.
func (a *app) Size() (int, int) { return a.w, a.h }

// Paint implements win.Callback (unused; redraw pushes directly).
func (a *app) Paint(w, h int) []byte { return nil }

// OnMove implements win.Callback: drag repositions, saved on release.
func (a *app) OnMove(x, y int) {
	pill := widget.PillBounds(a.view)
	x, y = clampWorkPos(x, y, a.w, a.h, win.WorkArea())
	a.x, a.y = x+pill.Min.X, y+pill.Min.Y
	win.Resize(x, y, a.w, a.h)
	a.settings.X, a.settings.Y = a.x, a.y
	a.settings.HasPos = true
}

func (a *app) OnDragEnd() { saveSettings(a.settings) }

func (a *app) OnDPIChanged(scale float32) { a.scale = scale; a.redraw() }

func (a *app) OnPointerClick(x, y int) {
	if image.Pt(x, y).In(widget.RefreshBounds(a.view)) {
		a.poller.Refresh(true)
		return
	}
	// A reset row toggles its own countdown/clock instead of (un)pinning the
	// panel; ResetRowBounds is empty when closed, absent, or reset-less, so
	// those clicks still fall through to OnClick below.
	for _, id := range visibleAgents(a.settings) {
		if image.Pt(x, y).In(widget.ResetRowBounds(a.view, id)) {
			if a.resetClock == nil {
				a.resetClock = map[string]bool{}
			}
			a.resetClock[id] = !a.resetClock[id]
			a.redraw()
			return
		}
	}
	a.OnClick()
}

func (a *app) OnPointerMove(x, y int) {
	hovered := image.Pt(x, y).In(widget.RefreshBounds(a.view))
	if a.refreshHovered != hovered {
		a.refreshHovered = hovered
		a.redraw()
	}
}

func (a *app) PointerHand() bool { return a.refreshHovered }

func (a *app) reveal() {
	v := a.buildView()
	v.Open = false
	w, h := widget.Size(v)
	a.x, a.y = initialPosition(settings{X: a.x, Y: a.y, HasPos: true}, w, h, a.scale, win.WorkArea())
	a.open, a.pinned = true, true
	a.redraw()
	saveSettings(a.settings)
}

// OnClick implements win.Callback: pins/unpins the panel.
func (a *app) OnClick() {
	a.pinned = !a.pinned
	a.open = a.pinned
	a.redraw()
}

// OnHover implements win.Callback: hover opens, leave closes unless pinned.
func (a *app) OnHover(open bool) {
	if a.pinned {
		return
	}
	if a.open != open {
		a.open = open
		a.redraw()
	}
}

// OnMenu implements win.Callback.
func (a *app) OnMenu(id int) {
	switch id {
	case menuReveal:
		a.reveal()
		return
	case menuChanged:
		a.checkedAt = time.Now()
		a.redraw()
		return
	case menuClaudeSnapshot:
		a.poller.RefreshOne(quota.ItemClaude)
		return
	case 101: // 30 s poll timer
		a.poller.Refresh(false)
		// Countdown strings and "Checked N min ago" age between readings, so
		// the tick repaints even when no new reading lands.
		a.redraw()
		return
	case menuRefresh:
		a.poller.Refresh(true)
		return
	case menuAutostart:
		_ = win.SetAutostart(!win.AutostartEnabled())
		return
	case menuQuit:
		win.Quit()
		return
	}
	if id >= menuAgentBase {
		i := id - menuAgentBase
		if i >= 0 && i < len(quota.ItemAll) {
			id := quota.ItemAll[i]
			a.visibilityMu.Lock()
			a.settings.Visible[id] = !a.settings.Visible[id]
			a.visibilityMu.Unlock()
			saveSettings(a.settings)
			a.poller.Refresh(false)
			a.redraw()
		}
	}
}

// Menu implements win.Callback.
func (a *app) Menu() []win.MenuItem {
	items := []win.MenuItem{{ID: menuRefresh, Label: "Refresh"}}
	for i, id := range quota.ItemAll {
		items = append(items, win.MenuItem{
			ID:      menuAgentBase + i,
			Label:   quota.ItemTitle(id),
			Checked: a.settings.Visible[id],
		})
	}
	auto := "Start with Windows"
	if win.AutostartEnabled() {
		auto = "✓ " + auto
	}
	items = append(items,
		win.MenuItem{Sep: true},
		win.MenuItem{ID: menuAutostart, Label: auto, Checked: win.AutostartEnabled()},
		win.MenuItem{ID: menuQuit, Label: "Quit"},
	)
	return items
}
