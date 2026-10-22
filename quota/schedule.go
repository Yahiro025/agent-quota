package quota

// Local and API readers poll every 30 seconds. Kiro and Antigravity retain five
// minutes between CLI starts because their process trees are substantially heavier.
// Manual refresh bypasses the interval but never overlaps a provider's current read.

import (
	"slices"
	"sync"
	"time"

	"agent-quota/internal/core"
)

const Every = 5 * time.Minute

// Tick is OwlApp's DispatcherTimer.
const Tick = 30 * time.Second

type stamped struct {
	r  Reading
	at time.Time
}

// Book is the readings and which reads are under way. Pure bookkeeping, so it is tested
// with a clock of its own. Unlike Hover, the last good reading is kept beside the
// latest attempt: a failed re-read goes Stale instead of wiping a good number.
type Book struct {
	readings map[string]stamped
	good     map[string]stamped
	busy     map[string]bool
	next     map[string]time.Time
}

func NewBook() *Book {
	return &Book{readings: map[string]stamped{}, good: map[string]stamped{}, busy: map[string]bool{}, next: map[string]time.Time{}}
}

// State is what the widget shows: Waiting (no reading), OK, Stale (latest failed,
// earlier ok), Failed (every read failed).
type State int

const (
	Waiting State = iota
	OK
	Stale
	Failed
)

// StateOf reports the display state plus the reading to show (last good when Stale)
// and the latest error detail ("" unless Stale/Failed).
func (b *Book) StateOf(id string) (State, Reading, string, bool) {
	latest, ok := b.readings[id]
	if !ok {
		return Waiting, Reading{}, "", false
	}
	if latest.r.OK() {
		return OK, latest.r, "", true
	}
	if g, ok := b.good[id]; ok {
		return Stale, g.r, latest.r.Detail, true
	}
	return Failed, Reading{}, latest.r.Detail, true
}

// Stale reports whether id's latest read failed while an earlier one was good.
func (b *Book) Stale(id string) bool {
	latest, ok := b.readings[id]
	if !ok || latest.r.OK() {
		return false
	}
	_, ok = b.good[id]
	return ok
}

// LastGood is the newest good reading, for the "last good N min ago" line.
func (b *Book) LastGood(id string) (Reading, time.Time, bool) {
	g, ok := b.good[id]
	return g.r, g.at, ok
}

// Refresh is RefreshQuotas: it drops what is off (true when that changed anything) and
// marks busy the ids to read now, in the order given.
func (b *Book) Refresh(on []string, force bool, now time.Time) (bool, []string) {
	dropped := false
	for k := range b.readings {
		if !slices.Contains(on, k) {
			delete(b.readings, k)
			delete(b.good, k)
			dropped = true
		}
	}
	for k := range b.good {
		if !slices.Contains(on, k) {
			delete(b.good, k)
			dropped = true
		}
	}
	var start []string
	for id := range b.next {
		if !slices.Contains(on, id) {
			delete(b.next, id)
		}
	}
	for _, id := range on {
		if b.RefreshOne(id, force, now) {
			start = append(start, id)
		}
	}
	return dropped, start
}

// RefreshOne marks one provider for an immediate read without pruning or starting any
// other provider. Status-line events use this so a Claude turn cannot fan out to all tools.
func (b *Book) RefreshOne(id string, force bool, now time.Time) bool {
	if b.busy[id] {
		return false
	}
	interval := Tick
	if id == ItemKiro || id == ItemAntigravity {
		interval = Every
	}
	if !force {
		due, ok := b.next[id]
		if !ok {
			if r, exists := b.readings[id]; exists {
				due = r.at.Add(interval)
			}
		}
		if now.Before(due) {
			return false
		}
	}
	b.busy[id] = true
	// Scheduling from completion would skip the next tick after any nonzero
	// read duration, turning a 30-second interval into 60 seconds.
	b.next[id] = now.Add(interval)
	return true
}

// Finished: a read finished; it is kept only while the quota is still switched on. True
// when the reading was kept (the views redraw). Good readings also refresh the
// last-good slot; failures leave it so the widget can go Stale.
func (b *Book) Finished(id string, r Reading, stillOn bool, now time.Time) bool {
	delete(b.busy, id)
	if !stillOn {
		return false
	}
	b.readings[id] = stamped{r, now}
	if r.OK() {
		b.good[id] = stamped{r, now}
	}
	return true
}

func (b *Book) Busy(id string) bool { return b.busy[id] }

// Poller is the book, the reader and a callback for changes: what OwlApp's quota half is.
type Poller struct {
	mu      sync.Mutex
	book    *Book
	read    func(id string) Reading
	isOn    func(id string) bool
	changed func()
}

func NewPoller(read func(string) Reading, isOn func(string) bool, changed func()) *Poller {
	return &Poller{book: NewBook(), read: read, isOn: isOn, changed: changed}
}

// SystemPoller has the real readers.
func SystemPoller(isOn func(string) bool, changed func()) *Poller {
	return SystemPollerWith(isOn, changed, func(KiroUsage) {})
}

// SystemPollerWith has the real readers, and Kiro's raw credits handed on as well, told on
// the poll's goroutine for every good Kiro reading. They don't ride on Reading: a field
// there would be carried by the other tools too, who never fill it. The Kiro reader sees
// them where it parses the report, and the other tools are read as before.
func SystemPollerWith(isOn func(string) bool, changed func(), onUsage func(KiroUsage)) *Poller {
	return NewPoller(func(id string) Reading {
		if id != ItemKiro {
			return ByID(id)
		}
		r, usage := KiroRead()
		if usage != nil {
			onUsage(*usage)
		}
		return r
	}, isOn, changed)
}

func (p *Poller) Reading(id string) (Reading, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.book.readings[id]
	return r.r, ok
}

// Display is what the widget paints: state + reading to show + error + presence.
func (p *Poller) Display(id string) (State, Reading, string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.book.StateOf(id)
}

func (p *Poller) Busy(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.book.Busy(id)
}

// LastGoodAt is the newest good reading and its time, for "last good N min ago".
func (p *Poller) LastGoodAt(id string) (Reading, time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.book.LastGood(id)
}

func (p *Poller) Refresh(force bool) {
	var on []string
	for _, id := range ItemAll {
		if p.isOn(id) {
			on = append(on, id)
		}
	}
	p.mu.Lock()
	dropped, start := p.book.Refresh(on, force, time.Now())
	p.mu.Unlock()
	if dropped {
		p.changed()
	}
	p.run(start)
}

// RefreshOne requests a read of just id. It leaves all other providers' readings and
// in-flight work alone, and honors the same per-provider one-flight guard as Refresh.
func (p *Poller) RefreshOne(id string) {
	if !p.isOn(id) {
		p.mu.Lock()
		changed := false
		if _, ok := p.book.readings[id]; ok {
			delete(p.book.readings, id)
			changed = true
		}
		if _, ok := p.book.good[id]; ok {
			delete(p.book.good, id)
			changed = true
		}
		p.mu.Unlock()
		if changed {
			p.changed()
		}
		return
	}
	p.mu.Lock()
	start := []string{}
	if p.book.RefreshOne(id, true, time.Now()) {
		start = append(start, id)
	}
	p.mu.Unlock()
	p.run(start)
}

func (p *Poller) run(start []string) {
	for _, id := range start {
		go func() {
			r := p.read(id)
			if !r.OK() {
				core.Logf("quota %s: %s", id, r.Detail)
			}
			stillOn := p.isOn(id)
			p.mu.Lock()
			kept := p.book.Finished(id, r, stillOn, time.Now())
			p.mu.Unlock()
			if kept {
				p.changed()
			}
		}()
	}
}
