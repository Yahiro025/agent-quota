package quota

// The service session turns parent lines into poller commands and snapshot
// events, per docs/superpowers/specs/2026-10-22-bloom-agent-quota-design.md.
// NewPoller fixes the changed callback before a Service exists, so callers
// hand it s.OnChanged; that callback runs on a poller read goroutine, which
// is why every write to the pipe goes through the mutex.

import (
	"bufio"
	"io"
	"slices"
	"strings"
	"sync"
)

// Service is one session: a poller plus the writer Serve installs. The writer
// is nil outside Serve so a change before (or after) a session writes nothing.
//
// Visibility is the parent's hidden set. The poller asks it On from its read
// goroutines while the session loop replaces the set, so it carries its own
// lock. A session that never sees an agents line starts with nothing hidden.
type Visibility struct {
	mu     sync.RWMutex
	hidden map[string]bool
}

func NewVisibility() *Visibility {
	return &Visibility{hidden: map[string]bool{}}
}

// On reports whether id should be polled and shown.
func (v *Visibility) On(id string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return !v.hidden[id]
}

// SetHidden replaces the hidden set. Ids outside ItemAll are ignored, so a
// newer parent may name an agent this build does not have.
func (v *Visibility) SetHidden(ids []string) {
	next := make(map[string]bool, len(ids))
	for _, id := range ids {
		if slices.Contains(ItemAll[:], id) {
			next[id] = true
		}
	}
	v.mu.Lock()
	v.hidden = next
	v.mu.Unlock()
}

type Service struct {
	p       *Poller
	visible *Visibility
	mu      sync.Mutex
	w       io.Writer
}

func NewService(p *Poller) *Service {
	return NewServiceWith(p, NewVisibility())
}

// NewServiceWith shares the parent's visibility set with the poller's isOn, so
// an agents line filters snapshots and stops the reads themselves.
func NewServiceWith(p *Poller, visible *Visibility) *Service {
	return &Service{p: p, visible: visible}
}

// Handle answers one parent line with its reply events; done means shutdown.
// Decode failures answer an error and keep the session running.
func (s *Service) Handle(line string) (events []ServiceEvent, done bool) {
	if strings.TrimSpace(line) == "" {
		return nil, false
	}
	req, err := DecodeRequest([]byte(line))
	if err != nil {
		return []ServiceEvent{{V: ServiceVersion, Op: "error", Message: err.Error()}}, false
	}
	switch req.Op {
	case "hello":
		events = []ServiceEvent{
			{V: ServiceVersion, Op: "ready", Agents: slices.Clone(ItemAll[:])},
			s.Snapshot(),
		}
		// The snapshot is captured first so an instant read cannot turn the
		// hello snapshot ok before the parent ever saw it waiting.
		s.p.Refresh(true)
	case "refresh":
		if req.ID == "all" {
			s.p.Refresh(true)
		} else {
			s.p.RefreshOne(req.ID)
		}
	case "agents":
		s.visible.SetHidden(req.Hidden)
		events = []ServiceEvent{s.Snapshot()}
		// Refresh prunes the agents that just went off and starts the ones
		// that came back on; the snapshot above already reflects the new set
		// even before those reads finish.
		s.p.Refresh(true)
	case "shutdown":
		return []ServiceEvent{{V: ServiceVersion, Op: "bye"}}, true
	}
	return events, false
}

// Snapshot is one snapshot event holding every agent, in ItemAll order.
func (s *Service) Snapshot() ServiceEvent {
	ev := ServiceEvent{V: ServiceVersion, Op: "snapshot", Readings: make([]ServiceAgent, 0, len(ItemAll))}
	for _, id := range ItemAll {
		if !s.visible.On(id) {
			continue
		}
		ev.Readings = append(ev.Readings, s.agent(id))
	}
	return ev
}

func (s *Service) agent(id string) ServiceAgent {
	state, r, errText, _ := s.p.Display(id)
	a := ServiceAgent{
		ID:         id,
		State:      serviceState(state),
		Busy:       s.p.Busy(id),
		Detail:     r.Detail,
		Err:        errText,
		Plan:       r.Plan,
		Summary:    r.Summary,
		ResetLabel: r.ResetLabel,
		Reset5h:    r.Reset5h,
		ObservedAt: r.ObservedAt,
	}
	// A failure (or a stale reading's latest error) carries the same recovery
	// hint the standalone shows, so the client can offer the fix in place.
	if state == Failed || state == Stale {
		a.Help = RecoveryHint(id, errText)
	}
	// Stale's displayed reading is the last good one, so its Used pointer is
	// exactly the kept value; Failed displays an empty reading and sends none.
	a.Used = r.Used
	if len(r.Windows) > 0 {
		a.Windows = make([]ServiceWindow, len(r.Windows))
		for i, w := range r.Windows {
			a.Windows[i] = ServiceWindow{Label: w.Label, Used: w.Used}
		}
	}
	if len(r.Groups) > 0 {
		a.Groups = make([]ServiceGroup, len(r.Groups))
		for i, g := range r.Groups {
			// ServiceGroup.Windows has no omitempty: a nil slice would encode
			// as null, so a group without windows sends an empty array.
			windows := make([]ServiceWindow, len(g.Windows))
			for j, w := range g.Windows {
				windows[j] = ServiceWindow{Label: w.Label, Used: w.Used}
			}
			a.Groups[i] = ServiceGroup{Name: g.Name, Windows: windows}
		}
	}
	if len(r.Details) > 0 {
		a.Details = make([]ServiceDetail, len(r.Details))
		for i, d := range r.Details {
			a.Details[i] = ServiceDetail{Label: d.Label, Value: d.Value, ResetAt: d.ResetAt, ResetText: d.ResetText}
		}
	}
	if _, at, ok := s.p.LastGoodAt(id); ok {
		a.LastGoodAt = at
	}
	return a
}

func serviceState(st State) string {
	switch st {
	case OK:
		return "ok"
	case Stale:
		return "stale"
	case Failed:
		return "failed"
	}
	return "waiting"
}

// OnChanged is the poller's changed callback: one fresh snapshot on the pipe.
// It runs on a read goroutine, so it shares the writer mutex with Serve.
func (s *Service) OnChanged() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		s.writeLocked(s.Snapshot())
	}
}

func (s *Service) writeLocked(e ServiceEvent) error {
	raw, err := EncodeEvent(e)
	if err != nil {
		return err
	}
	_, err = s.w.Write(append(raw, '\n'))
	return err
}

// Serve owns the session loop: it installs w as the pipe, then per line runs
// Handle and writes that line's events. Handle stays outside the lock because
// a refresh can run the changed callback — s.OnChanged, which takes s.mu — on
// this same goroutine, so holding s.mu across it would self-deadlock. Locking
// just the writes keeps a changed snapshot before or after a reply pair, never
// inside it. Serve returns after shutdown or when the parent's input ends.
func Serve(r io.Reader, w io.Writer, s *Service) error {
	s.mu.Lock()
	s.w = w
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.w = nil
		s.mu.Unlock()
	}()

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		events, done := s.Handle(sc.Text())
		s.mu.Lock()
		var werr error
		for _, e := range events {
			if werr = s.writeLocked(e); werr != nil {
				break
			}
		}
		s.mu.Unlock()
		if werr != nil {
			return werr
		}
		if done {
			return nil
		}
	}
	return sc.Err()
}
