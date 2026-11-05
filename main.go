package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
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
	X, Y          int
	HasPos        bool
	Visible       map[string]bool
	Compact       bool
	Selected      string
	LayoutVersion int
	Docked        bool
}

func loadSettings() settings {
	s := settings{Visible: map[string]bool{}, Selected: quota.ItemClaude}
	for _, id := range quota.ItemAll {
		s.Visible[id] = true
	}
	b, err := os.ReadFile(core.SettingsFile())
	if err != nil {
		return s
	}
	var raw struct {
		X, Y          *int
		Visible       map[string]bool
		Compact       bool
		Selected      string
		LayoutVersion int  `json:"layout_version"`
		Docked        bool `json:"docked"`
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
	s.Docked = raw.Docked
	s.Compact = raw.Compact
	s.LayoutVersion = raw.LayoutVersion
	if raw.Selected != "" {
		s.Selected = raw.Selected
	}
	return s
}

func saveSettings(s settings) {
	raw := map[string]any{"x": s.X, "y": s.Y, "visible": s.Visible, "compact": s.Compact, "selected": s.Selected, "layout_version": s.LayoutVersion, "docked": s.Docked}
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

func detailResetLabel(row quota.Detail, now time.Time, clock bool) string {
	if row.ResetAt.IsZero() {
		if row.ResetText != "" {
			return "resets " + row.ResetText
		}
		return ""
	}
	if !row.ResetAt.After(now) {
		return "resets at " + quota.FormatLocal(row.ResetAt, "d MMM HH:mm")
	}
	if clock {
		panel, _ := resetLabels(row.ResetAt, now, true)
		return strings.TrimPrefix(panel, "5h ")
	}
	mins := int(row.ResetAt.Sub(now).Minutes())
	if days := mins / (24 * 60); days > 0 {
		return fmt.Sprintf("resets in %dd %dh %dm", days, mins/60%24, mins%60)
	}
	panel, _ := resetLabels(row.ResetAt, now, false)
	return strings.TrimPrefix(panel, "5h ")
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
			// Frozen windows and reset labels keep screenshot review independent
			// of the clock and the user's live account readings.
			{ID: "claude", State: quota.OK, Used: 70, Plan: "Pro", Windows: []quota.Window{{Label: "5h", Used: 19}, {Label: "week", Used: 70}}, Reset: "5h resets in 12m", PillReset: "12m"},
			{ID: "kiro", State: quota.OK, Used: 3, Plan: "Free", Summary: "1.66 of 50 credits", ResetLabel: "1 Nov"},
			{ID: "codex", State: quota.OK, Used: 14, Windows: []quota.Window{{Label: "5h", Used: 0}, {Label: "week", Used: 14}}},
			{ID: "cursor", State: quota.OK, Used: 66, Plan: "Pro", Summary: "Plan usage", ResetLabel: "28 Oct"},
			{ID: "copilot", State: quota.OK, Used: 13, Plan: "Individual", Summary: "Premium interactions", ResetLabel: "1 Nov"},
			{ID: "devin", State: quota.OK, Used: 38, Plan: "Pro", Windows: []quota.Window{{Label: "day", Used: 0}, {Label: "week", Used: 38}}, ResetLabel: "16:00"},
			{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Groups: []quota.Group{{Name: "Gemini", Windows: []quota.Window{{Label: "5h", Used: 0}, {Label: "week", Used: 2}}}, {Name: "Claude / GPT", Windows: []quota.Window{{Label: "5h", Used: 0}, {Label: "week", Used: 10}}}}, Reset: "5h resets in 4h 59m", PillReset: "4h59m"},
		}, Checked: "Checked just now", Selected: quota.ItemClaude},
	}
	fixtures := map[string][]widget.Detail{
		quota.ItemClaude:      {{Label: "5h", Value: "19% used", Reset: "resets in 12m"}, {Label: "Week", Value: "70% used", Reset: "resets in 3d 6h 0m"}},
		quota.ItemKiro:        {{Label: "Credits", Value: "1.66 of 50 credits", Reset: "resets 2026-11-01"}},
		quota.ItemCodex:       {{Label: "5h", Value: "0% used"}, {Label: "Week", Value: "14% used", Reset: "resets in 4d 2h 0m"}},
		quota.ItemCursor:      {{Label: "Plan total", Value: "66% used", Reset: "resets at 28 Oct 08:00"}, {Label: "Auto", Value: "20% used", Reset: "resets at 28 Oct 08:00"}, {Label: "API", Value: "80% used", Reset: "resets at 28 Oct 08:00"}, {Label: "Plan", Value: "1234 of 2000 used", Reset: "resets at 28 Oct 08:00"}, {Label: "Overall", Value: "1500 of 3000 used", Reset: "resets at 28 Oct 08:00"}, {Label: "Team pooled", Value: "4000 of 10000 used", Reset: "resets at 28 Oct 08:00"}},
		quota.ItemCopilot:     {{Label: "premium", Value: "13% used · 26 of 200 used · 174 remaining", Reset: "resets 2026-11-01"}, {Label: "chat", Value: "Unlimited", Reset: "resets 2026-11-01"}, {Label: "completions", Value: "Unlimited", Reset: "resets 2026-11-01"}},
		quota.ItemDevin:       {{Label: "Day", Value: "0% used", Reset: "resets in 8h 0m"}, {Label: "Week", Value: "38% used", Reset: "resets in 4d 8h 0m"}},
		quota.ItemAntigravity: {{Label: "Gemini Models / 5h", Value: "0% used", Reset: "resets in 4h 59m"}, {Label: "Gemini Models / Week", Value: "2% used", Reset: "resets at 23 Oct 08:00"}, {Label: "Claude and GPT models / Week", Value: "10% used", Reset: "resets at 22 Oct 08:00"}},
	}
	base := states["pill"]
	for i := range base.Items {
		base.Items[i].Details = fixtures[base.Items[i].ID]
		if base.Items[i].ID == quota.ItemClaude || base.Items[i].ID == quota.ItemCodex {
			base.Items[i].Observed = "16 Oct 08:00"
		}
	}
	states["pill"] = base
	// panel and empty are built only here: literals in the map above would be
	// dead code the two assignments overwrite anyway.
	panel := states["pill"]
	panel.Open = true
	states["panel"] = panel
	codex := panel
	codex.Selected = quota.ItemCodex
	codex.Items = append([]widget.Item(nil), panel.Items...)
	codex.Items[2] = widget.Item{ID: quota.ItemCodex, State: quota.OK, Used: 68,
		Windows: []quota.Window{{Label: "5h", Used: 68}, {Label: "week", Used: 37}},
		Details: []widget.Detail{{Label: "5h", Value: "68% used", Reset: "resets in 33m"}, {Label: "Week", Value: "37% used", Reset: "resets at 23 Oct 08:00"}}, Observed: "16 Oct 08:00",
		Reset: "5h resets in 33m", PillReset: "33m"}
	for label, scale := range map[string]float32{"100": 1, "125": 1.25, "150": 1.5, "200": 2} {
		codex.Scale = scale
		states["codex-windows-"+label] = codex
		narrow := codex
		narrow.Items = []widget.Item{codex.Items[2]}
		states["codex-windows-narrow-"+label] = narrow
		for _, it := range panel.Items {
			selected := panel
			selected.Scale, selected.Selected = scale, it.ID
			states["details-"+it.ID+"-"+label] = selected
			narrow := selected
			narrow.Items = []widget.Item{it}
			states["details-"+it.ID+"-narrow-"+label] = narrow
			stale := narrow
			stale.Items = append([]widget.Item(nil), narrow.Items...)
			stale.Items[0].State = quota.Stale
			stale.Items[0].Stale = "8 min ago"
			stale.Items[0].Err = "The latest usage check failed. Check the connection, then choose Refresh."
			stale.Items[0].Reset, stale.Items[0].PillReset = "", ""
			states["details-"+it.ID+"-stale-"+label] = stale
		}
	}
	states["empty"] = widget.View{Open: true}
	compact := panel
	compact.Compact = true
	states["compact"] = compact
	compact.Open = false
	states["compact-resting"] = compact
	compact.Floating = true
	states["compact-floating"] = compact
	floating := states["pill"]
	floating.Floating = true
	states["pill-floating"] = floating
	floating.Open = true
	states["panel-floating"] = floating
	groups := panel
	groups.GroupsOpen = true
	groups.Selected = quota.ItemAntigravity
	states["groups"] = groups
	missing := groups
	missing.Items = append([]widget.Item(nil), groups.Items...)
	for i := range missing.Items {
		if missing.Items[i].ID == quota.ItemAntigravity {
			missing.Items[i].Groups = append([]quota.Group(nil), missing.Items[i].Groups...)
			missing.Items[i].Groups[0].Windows = []quota.Window{{Label: "week", Used: 2}}
		}
	}
	states["groups-missing-window"] = missing
	issues := panel
	issues.Items = append([]widget.Item(nil), panel.Items...)
	issues.Items[0].State = quota.Stale
	issues.Items[0].Err = "Update failed"
	issues.Items[0].Stale = "8 min ago"
	issues.Items[0].Reset, issues.Items[0].PillReset = "", ""
	issues.Items[5] = widget.Item{ID: quota.ItemDevin, State: quota.Failed, Err: "Sign-in expired.", Help: "Open Devin and sign in, then choose Refresh."}
	states["issues"] = issues
	failed := issues
	failed.Selected = quota.ItemDevin
	states["failed"] = failed
	failed.RecoveryOpen = quota.ItemDevin
	states["recovery"] = failed
	checking := panel
	checking.Items = append([]widget.Item(nil), panel.Items...)
	for i := range checking.Items {
		checking.Items[i] = widget.Item{ID: checking.Items[i].ID, State: quota.Waiting}
	}
	checking.Busy = true
	states["checking"] = checking
	panel.Scale = 1.5
	states["panel-150"] = panel
	panel.Scale = 1.25
	states["panel-125"] = panel
	panel.Scale = 2
	states["panel-200"] = panel
	pill := states["pill"]
	pill.Scale = 1.5
	states["pill-150"] = pill
	focusStates := map[string]widget.View{}
	for name, v := range states {
		if !v.Open {
			continue
		}
		f := v
		f.Focus = true
		f.FocusMaxHeight = 620
		focusStates["focus-"+name] = f
	}
	for name, v := range focusStates {
		states[name] = v
	}
	for name, v := range states {
		v.Seam = true
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		img := widget.Render(v)
		_ = png.Encode(f, img)
		f.Close()
		if name == "panel-150" {
			for label, ink := range map[string]color.RGBA{"panel-on-gray": {R: 42, G: 46, B: 52, A: 255}, "panel-on-light": {R: 241, G: 241, B: 241, A: 255}} {
				proof := image.NewRGBA(img.Bounds())
				draw.Draw(proof, proof.Bounds(), image.NewUniform(ink), image.Point{}, draw.Src)
				draw.Draw(proof, proof.Bounds(), img, image.Point{}, draw.Over)
				out, err := os.Create(filepath.Join(dir, label+".png"))
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				_ = png.Encode(out, proof)
				out.Close()
			}
		}
	}
	return 0
}

func main() {
	printFlag := flag.Bool("print", false, "read each agent once and print results")
	quotaServiceFlag := flag.Bool("quota-service", false, "serve quota snapshots as JSON lines on stdin/stdout")
	shotDir := flag.String("shot", "", "write pill/panel PNGs to DIR")
	statuslineFlag := flag.Bool("claude-statusline", false, "receive Claude Code statusLine input")
	installClaudeFeedFlag := flag.Bool("install-claude-feed", false, "connect Claude Code statusLine to Agent Quota")
	installTaskbar := flag.String("install-taskbar", "", "install the native taskbar host from MANIFEST")
	taskbarStatus := flag.Bool("taskbar-status", false, "print native taskbar geometry and attachment state")
	flag.Parse()
	if *taskbarStatus {
		win.EnableDPIAwareness()
		data, _ := json.MarshalIndent(win.TaskbarStatus(), "", "  ")
		fmt.Println(string(data))
		return
	}
	if *installTaskbar != "" {
		if err := win.InstallTaskbar(*installTaskbar); err != nil {
			core.Logf("install taskbar: %v", err)
			os.Exit(1)
		}
		s := loadSettings()
		s.Docked = true
		saveSettings(s)
		return
	}
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
	// The service speaks on stdin/stdout: no window, no detach, no
	// Local\AgentQuotaRunning mutex, so it can run beside the native widget.
	if *quotaServiceFlag {
		if err := runQuotaService(os.Stdin, os.Stdout); err != nil {
			os.Exit(1)
		}
		return
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

// quotaServiceSelected reports whether args carries the exact --quota-service
// token. It scans instead of calling flag.Parse so tests can check selection
// without touching the process FlagSet.
func quotaServiceSelected(args []string) bool {
	for _, a := range args {
		if a == "--quota-service" {
			return true
		}
	}
	return false
}

// runQuotaService is the headless session: every agent reads on the 30-second
// tick, and each finished read pushes one snapshot line on w through
// s.OnChanged. NewPoller fixes the changed callback before a Service can
// exist, so the closure guards the nil and s is assigned before any Refresh
// can run it. Serve returns on shutdown or when the parent's input ends; the
// ticker stops with it.
func runQuotaService(r io.Reader, w io.Writer) error {
	var s *quota.Service
	visible := quota.NewVisibility()
	p := quota.SystemPoller(visible.On, func() {
		if s != nil {
			s.OnChanged()
		}
	})
	s = quota.NewServiceWith(p, visible)
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(quota.Tick)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				p.Refresh(false)
			case <-done:
				return
			}
		}
	}()
	err := quota.Serve(r, w, s)
	close(done)
	if err != nil {
		core.Logf("quota service: %v", err)
	}
	return err
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
	menuRefresh    = 1
	menuAutostart  = 2
	menuQuit       = 3
	menuChanged    = 4
	menuReveal     = 5
	menuDocked     = 8
	menuCompact    = 7
	menuAgentBase  = 10
	menuSelectBase = 1000
)

type app struct {
	taskbar                                     *win.TaskbarBridge
	docked, nativeDrag, windowDrag, dockPreview bool
	// fullscreen hides the floating widget while a fullscreen app owns the
	// primary monitor. fullscreenCheck lets tests drive the maintenance tick
	// without a real foreground fullscreen window; nil asks the window layer.
	fullscreen                bool
	fullscreenCheck           func() bool
	nativeOffset, floatAnchor image.Point
	focusScroll               int
	visibilityMu              sync.RWMutex
	settings                  settings
	poller                    *quota.Poller
	open                      bool
	pinned                    bool
	checkedAt                 time.Time
	w, h                      int
	x, y                      int
	scale                     float32
	hovered                   widget.Target
	groupsOpen                bool
	recoveryOpen              string
	view                      widget.View
	dragAnchor                image.Point
	// resetClock flips a reset row between countdown and wall clock for that
	// agent's panel and pill strings. In-memory only (never in settings.json):
	// a fresh launch shows countdowns again. UI thread only, like open/pinned,
	// so no mutex.
	resetClock map[string]bool
}

func runWindow() {
	win.EnableDPIAwareness()
	s := migrateSeamSettings(loadSettings())
	a := &app{settings: s, checkedAt: time.Now(), scale: win.DPIScale(), resetClock: map[string]bool{}}
	a.poller = quota.SystemPoller(a.isOn, a.changed)
	initialView := widget.View{Items: make([]widget.Item, len(visibleAgents(s))), Scale: a.scale, Compact: s.Compact, Seam: true}
	pw, ph := widget.Size(initialView)
	s.X, s.Y = initialPillPosition(s, initialView, win.SeamWorkArea())
	s.HasPos = true
	a.x, a.y, a.w, a.h = s.X, s.Y, pw, ph
	a.settings = s
	saveSettings(s)
	if err := win.Loop(func() error {
		if err := win.Create("AgentQuota", a.x, a.y, a); err != nil {
			return err
		}
		a.taskbar = win.NewTaskbarBridge()
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

func initialPillPosition(s settings, v widget.View, work image.Rectangle) (int, int) {
	anchor := widget.PillBounds(v)
	if s.HasPos {
		s.X -= anchor.Min.X
		s.Y -= anchor.Min.Y
	}
	w, h := widget.Size(v)
	if v.Seam && (!s.HasPos || !image.Rect(s.X, s.Y, s.X+w, s.Y+h).In(work)) {
		x, y := clampWorkPos(work.Min.X+(work.Dx()-w)/2, work.Min.Y, w, h, work)
		return x + anchor.Min.X, y + anchor.Min.Y
	}
	scale := v.Scale
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		scale = 1
	}
	x, y := initialPosition(s, w, h, scale, work)
	return x + anchor.Min.X, y + anchor.Min.Y
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
			it.Plan, it.Summary, it.ResetLabel = show.Plan, show.Summary, show.ResetLabel
			it.Windows, it.Groups = show.Windows, show.Groups
			for _, row := range show.Details {
				it.Details = append(it.Details, widget.Detail{Label: row.Label, Value: row.Value, Reset: detailResetLabel(row, now, a.resetClock[id])})
			}
			if !show.ObservedAt.IsZero() {
				it.Observed = quota.FormatLocal(show.ObservedAt, "d MMM HH:mm")
			}
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
			it.Help = quota.RecoveryHint(id, err)
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
	v := widget.View{Items: items, Open: a.open, Checked: checkedAgo(a.checkedAt), Scale: a.scale, Hovered: a.hovered, Pinned: a.pinned, Compact: a.settings.Compact, Selected: a.settings.Selected, GroupsOpen: a.groupsOpen, RecoveryOpen: a.recoveryOpen, Seam: true}
	selectedVisible := false
	for _, it := range items {
		if it.ID == v.Selected {
			selectedVisible = true
			break
		}
	}
	if !selectedVisible && len(items) > 0 {
		v.Selected = items[0].ID
	}
	for _, it := range items {
		v.Busy = v.Busy || a.poller.Busy(it.ID)
	}
	work := win.WorkArea()
	v.Focus = a.docked
	v.DockPreview = a.dockPreview
	v.FocusScroll = a.focusScroll
	if a.docked {
		v.FocusMaxHeight = int(float32(work.Dy())/max(a.scale, 1)) - 48
		return v
	}
	// A notch needs the top screen edge to join; elsewhere it is a floating pill.
	// Seam reaches the monitor's top edge even when an appbar reserves it, and
	// the side/bottom bounds still come from the work area.
	seam := win.SeamWorkArea()
	v.Floating = a.y > seam.Min.Y
	_, h := widget.Size(v)
	v.Above = a.open && a.y-widget.PillBounds(v).Min.Y+h > seam.Max.Y
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
	win.EnableAgentNavigation(a.open && !a.nativeDrag && !a.windowDrag && len(a.view.Items) > 1)
	a.syncTaskbar()
	v := a.buildView()
	a.view = v
	win.EnableAgentNavigation(a.open && !a.nativeDrag && !a.windowDrag && len(v.Items) > 1)
	if a.drawTaskbarPopup(v) {
		return
	}
	// Fullscreen suppression happens before any sizing so the hidden window
	// keeps its last surface and can be restored without a repaint jump.
	if a.fullscreen {
		win.EnableAgentNavigation(false)
		win.SetVisible(false)
		return
	}
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
	x, y := clampWorkPos(a.x-pill.Min.X, a.y-pill.Min.Y, w, h, win.SeamWorkArea())
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

// isFullscreen asks the window layer whether a fullscreen app owns the primary
// monitor. Tests substitute fullscreenCheck; production never sets it.
func (a *app) isFullscreen() bool {
	if a.fullscreenCheck != nil {
		return a.fullscreenCheck()
	}
	return win.Fullscreen()
}

// Size implements win.Callback.
func (a *app) Size() (int, int) { return a.w, a.h }

// Paint implements win.Callback (unused; redraw pushes directly).
func (a *app) Paint(w, h int) []byte { return nil }

// Keep the mouse-down anchor stable when detaching adds top shadow padding.
func (a *app) OnDragStart() {
	a.dragAnchor = widget.PillBounds(a.view).Min
	a.floatAnchor = image.Pt(a.settings.X, a.settings.Y)
	a.windowDrag = true
}

// OnMove implements win.Callback: drag repositions, saved on release.
func (a *app) OnMove(x, y int) {
	a.updateDockPreview(win.CursorPosition())
	a.x, a.y = x+a.dragAnchor.X, y+a.dragAnchor.Y
	v := a.buildView()
	if v.Floating != a.view.Floating || v.Above != a.view.Above || v.DockPreview != a.view.DockPreview {
		a.redraw()
	} else {
		pill := widget.PillBounds(a.view)
		x, y = clampWorkPos(a.x-pill.Min.X, a.y-pill.Min.Y, a.w, a.h, win.SeamWorkArea())
		a.x, a.y = x+pill.Min.X, y+pill.Min.Y
		win.Resize(x, y, a.w, a.h)
	}
	a.settings.X, a.settings.Y = a.x, a.y
	a.settings.HasPos = true
}

func (a *app) OnDragEnd() { a.finishDockDrag() }

func (a *app) OnDPIChanged(scale float32) { a.scale = scale; a.redraw() }

func (a *app) OnPointerClick(x, y int) {
	a.windowDrag = false
	target := widget.Hit(a.view, image.Pt(x, y))
	switch target.Control {
	case widget.RefreshControl:
		a.poller.Refresh(true)
		a.redraw()
		return
	case widget.PinControl:
		a.pinned = !a.pinned
	case widget.CloseControl:
		a.open, a.pinned = false, false
	case widget.GroupsControl:
		a.groupsOpen = !a.groupsOpen
	case widget.RecoveryControl:
		if a.recoveryOpen == target.ID {
			a.recoveryOpen = ""
		} else {
			a.recoveryOpen = target.ID
		}
	case widget.ResetControl:
		if a.resetClock == nil {
			a.resetClock = map[string]bool{}
		}
		a.resetClock[target.ID] = !a.resetClock[target.ID]
	case widget.AgentControl:
		a.selectAgent(target.ID)
		a.open = true
	case widget.OpenControl:
		a.OnClick()
		return
	default:
		return
	}
	a.hovered = widget.Target{}
	a.redraw()
}

func (a *app) OnPointerMove(x, y int) {
	hovered := widget.Hit(a.view, image.Pt(x, y))
	if a.hovered != hovered {
		a.hovered = hovered
		a.redraw()
	}
}

func (a *app) PointerHand() bool           { return a.hovered.Control != widget.NoControl }
func (a *app) ContainsPoint(x, y int) bool { return widget.Contains(a.view, image.Pt(x, y)) }
func (a *app) CanDrag(x, y int) bool {
	return widget.CanDrag(a.view, image.Pt(x, y))
}

func (a *app) reveal() {
	if a.docked {
		a.open, a.pinned = true, true
		a.redraw()
		return
	}
	v := a.buildView()
	v.Open = false
	a.fullscreen = false
	a.x, a.y = initialPillPosition(settings{X: a.x, Y: a.y, HasPos: true}, v, win.SeamWorkArea())
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
	if a.pinned || a.docked {
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
		// Poller keeps CLI readers on their slower cadence and skips busy readers.
		a.poller.Refresh(false)
		// Countdown strings and "Checked N min ago" age between readings, so
		// the tick repaints even when no new reading lands.
		a.redraw()
		return
	case 104: // 2 s topmost and fullscreen maintenance tick
		if a.docked || a.windowDrag || a.nativeDrag {
			return
		}
		fs := a.isFullscreen()
		if fs != a.fullscreen {
			a.fullscreen = fs
			if fs && a.open && !a.pinned {
				a.open = false
				a.hovered = widget.Target{}
			}
			a.redraw()
			return
		}
		if !a.fullscreen {
			// Same-band appbars can re-order themselves; re-assert quietly and
			// only while the widget is on screen.
			win.RaiseTopmost()
		}
		return
	case menuRefresh:
		a.poller.Refresh(true)
		a.redraw()
		return
	case menuAutostart:
		_ = win.SetAutostart(!win.AutostartEnabled())
		return
	case menuDocked:
		a.settings.Docked = !a.settings.Docked
		a.open, a.pinned = false, false
		if !a.settings.Docked {
			a.x, a.y = a.settings.X, a.settings.Y
			a.scale = win.DPIScale()
		}
		a.redraw()
		saveSettings(a.settings)
		return
	case menuCompact:
		oldWidth := widget.PillBounds(a.view).Dx()
		a.settings.Compact = !a.settings.Compact
		a.x += (oldWidth - widget.PillBounds(a.buildView()).Dx()) / 2
		a.redraw()
		saveSettings(a.settings)
		return
	case menuQuit:
		if a.taskbar != nil {
			a.taskbar.Disconnect()
		}
		win.Quit()
		return
	}
	if id >= menuSelectBase && id < menuSelectBase+len(quota.ItemAll) {
		a.selectAgent(quota.ItemAll[id-menuSelectBase])
		a.redraw()
		return
	}
	if id >= menuAgentBase && id < menuAgentBase+len(quota.ItemAll) {
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
		win.MenuItem{ID: menuDocked, Label: "Dock in taskbar", Checked: a.settings.Docked},
		win.MenuItem{ID: menuCompact, Label: "Compact tab", Checked: a.settings.Compact},
		win.MenuItem{ID: menuAutostart, Label: auto, Checked: win.AutostartEnabled()},
		win.MenuItem{ID: menuQuit, Label: "Quit"},
	)
	if a.settings.Compact {
		items = append(items, win.MenuItem{Sep: true})
		available := visibleAgents(a.settings)
		for i, id := range quota.ItemAll {
			present := false
			for _, enabled := range available {
				if enabled == id {
					present = true
					break
				}
			}
			if present {
				items = append(items, win.MenuItem{ID: menuSelectBase + i, Label: "Show " + quota.ItemShort(id) + " in tab", Checked: a.view.Selected == id})
			}
		}
	}
	return items
}
