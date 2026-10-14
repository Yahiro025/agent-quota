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
	"strconv"
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

// formatCountdown renders a reset countdown. Hours are omitted when zero so
// short waits read "12m left". Day-plus waits collapse to days and remainder
// hours with minutes dropped so week windows read "5d 20h left". A
// non-positive duration means the reset already passed, so "" asks the caller
// to fall back to the wall-clock string instead of inventing a "resetting…"
// state. Floored to whole minutes so the panel never flickers on seconds.
func formatCountdown(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	mins := int(d.Minutes())
	h := mins / 60
	m := mins % 60
	if h >= 24 {
		return fmt.Sprintf("%dd %dh left", h/24, h%24)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm left", h, m)
	}
	return fmt.Sprintf("%dm left", m)
}

// resetMonth is FormatLocal's English MMM table, repeated here so the panel can
// read back the "d MMM HH:mm" tail it already printed without touching quota/.
var resetMonth = map[string]time.Month{
	"Jan": time.January, "Feb": time.February, "Mar": time.March,
	"Apr": time.April, "May": time.May, "Jun": time.June,
	"Jul": time.July, "Aug": time.August, "Sep": time.September,
	"Oct": time.October, "Nov": time.November, "Dec": time.December,
}

func parseHM(s string) (int, int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	if len(parts[0]) == 0 || len(parts[0]) > 2 || len(parts[1]) != 2 {
		return 0, 0, false
	}
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return 0, 0, false
			}
		}
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// parseResetClock reads back the wall-clock tail ("HH:mm" or "d MMM HH:mm") in
// local time. HH:mm is always the reading's own day, so a time at or before
// now means the reset just passed rather than tomorrow: rolling forward would
// hide that and show a ~24h countdown. A "d MMM" date tries this year first
// and only rolls to next year when that lands within a week, so a stale
// December reading never becomes an 11-month countdown.
func parseResetClock(rest string, now time.Time) (time.Time, bool) {
	rest = strings.TrimSpace(rest)
	if h, m, ok := parseHM(rest); ok {
		return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, time.Local), true
	}
	fields := strings.Fields(rest)
	if len(fields) != 3 {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(fields[0])
	if err != nil || day < 1 || day > 31 {
		return time.Time{}, false
	}
	mon, ok := resetMonth[fields[1]]
	if !ok {
		return time.Time{}, false
	}
	h, m, ok := parseHM(fields[2])
	if !ok {
		return time.Time{}, false
	}
	base := time.Date(now.Year(), mon, day, h, m, 0, 0, time.Local)
	if base.Month() != mon || base.Day() != day {
		return time.Time{}, false
	}
	if base.After(now) {
		return base, true
	}
	next := time.Date(now.Year()+1, mon, day, h, m, 0, 0, time.Local)
	if next.Month() == mon && next.Day() == day && next.After(now) && next.Sub(now) <= 8*24*time.Hour {
		return next, true
	}
	return base, true
}

// resetStrings splits a codex/claude Detail tail into its countdown and clock
// forms. The clock reuses the trailing "resets ..." token byte-for-byte so it
// can never disagree with Detail; the countdown parses that same token back to
// an instant (the Reading carries no unix, and re-reading disk/network on the
// UI thread would jank the drag). No trailing token means no usable reset, so
// both stay empty and the row keeps its pin behaviour.
func resetStrings(detail string, now time.Time) (countdown, clock string) {
	idx := strings.LastIndex(detail, "resets ")
	if idx < 0 {
		return "", ""
	}
	clock = strings.TrimSpace(detail[idx:])
	if strings.Contains(clock[len("resets "):], "·") {
		return "", ""
	}
	rest := strings.TrimSpace(clock[len("resets "):])
	t, ok := parseResetClock(rest, now)
	if !ok {
		return "", clock
	}
	if !t.After(now) {
		return "", clock
	}
	if cd := formatCountdown(t.Sub(now)); cd != "" {
		return cd, clock
	}
	return "", clock
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
			{ID: "claude", State: quota.OK, Used: 42, Detail: "Pro · 5h 42% · week 31%"},
			{ID: "kiro", State: quota.OK, Used: 75, Detail: "KIRO PRO · 21 of 50 credits"},
			{ID: "codex", State: quota.OK, Used: 95, Detail: "5h 95% · week 40%"},
			{ID: "cursor", State: quota.Failed, Err: "Sign in to Cursor first."},
			{ID: "copilot", State: quota.OK, Used: 13, Detail: "Individual · premium 26/200"},
			{ID: "devin", State: quota.Stale, Used: 62, Err: "Couldn't reach Devin's server.", Stale: "3 min"},
			{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Detail: "Gemini Models · 90% remaining"},
		}, Checked: "Checked 2 min ago"},
		"panel": {Open: true, Items: []widget.Item{
			{ID: "claude", State: quota.OK, Used: 42, Detail: "Pro · 5h 42% · week 31%"},
			{ID: "copilot", State: quota.OK, Used: 13, Detail: "Individual · premium 26/200"},
			{ID: "devin", State: quota.Stale, Used: 62, Err: "Couldn't reach Devin's server.", Stale: "3 min"},
			{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Detail: "Gemini Models · 90% remaining"},
		}, Checked: "Checked 2 min ago"},
		"empty": {Items: []widget.Item{
			{ID: "claude", State: quota.Waiting, Detail: "Checking…"},
		}, Open: true, Checked: "Checking…"},
	}
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
	// resetClock flips a codex/claude panel row between countdown and wall
	// clock. In-memory only (never in settings.json): a fresh launch shows
	// countdowns again. UI thread only, like open/pinned, so no mutex.
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
		// Only OK codex/claude rows carry a 5h-window reset; Stale keeps its
		// error line uncluttered and Failed/Waiting have no reset to toggle.
		// The toggled string widget appends is the single reset display, so a
		// usable reset strips Detail's trailing " · resets ..." tail from this
		// row copy (show.Detail upstream stays untouched). Without a usable
		// reset Detail stays byte-identical.
		if st == quota.OK && (id == quota.ItemCodex || id == quota.ItemClaude) {
			if cd, clock := resetStrings(show.Detail, now); clock != "" {
				it.ResetCountdown = cd
				it.ResetClock = clock
				it.ShowClock = a.resetClock[id]
				if j := strings.LastIndex(it.Detail, " · resets "); j >= 0 {
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
		if id != quota.ItemCodex && id != quota.ItemClaude {
			continue
		}
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
