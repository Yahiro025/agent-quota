package quota

// The quota service protocol: one compact JSON object per line between Bloom
// (parent stdin) and this helper (child stdout), per
// docs/superpowers/specs/2026-10-22-bloom-agent-quota-design.md. DecodeRequest
// validates a parent line; EncodeEvent produces one child line without the
// newline, which the writer owns as framing. The pipe carries usage, freshness,
// errors and reset instants only — never anything from a credential store.

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// ServiceVersion is the protocol's "v". A parent speaking another version gets
// an error rather than a misread line.
const ServiceVersion = 1

// ServiceRequest is a parent-to-child line: hello, refresh (id "all" or one
// agent), or shutdown. Shutdown carries no id.
type ServiceRequest struct {
	V  int    `json:"v"`
	Op string `json:"op"`
	ID string `json:"id,omitempty"`
}

// ServiceWindow is one source quota boundary, kept as numbers so the panel does
// not reconstruct percentages from display strings.
type ServiceWindow struct {
	Label string  `json:"label"`
	Used  float64 `json:"used"`
}

// ServiceGroup keeps a model group together with its windows.
type ServiceGroup struct {
	Name    string          `json:"name"`
	Windows []ServiceWindow `json:"windows"`
}

// ServiceDetail is one reported quota fact and its own reset: ResetAt when the
// source gave an instant, ResetText for dates reported without a time zone.
type ServiceDetail struct {
	Label     string    `json:"label"`
	Value     string    `json:"value"`
	ResetAt   time.Time `json:"resetAt,omitzero"`
	ResetText string    `json:"resetText,omitempty"`
}

// ServiceAgent is one reading inside a snapshot event. Used is a pointer so a
// reported 0% is encoded while no percentage at all leaves the field absent;
// the times and display strings are omitted while empty.
type ServiceAgent struct {
	ID         string          `json:"id"`
	State      string          `json:"state"`
	Busy       bool            `json:"busy"`
	Used       *float64        `json:"used,omitempty"`
	Detail     string          `json:"detail,omitempty"`
	Err        string          `json:"err,omitempty"`
	Help       string          `json:"help,omitempty"`
	Plan       string          `json:"plan,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	ResetLabel string          `json:"resetLabel,omitempty"`
	Reset5h    time.Time       `json:"reset5h,omitzero"`
	ObservedAt time.Time       `json:"observedAt,omitzero"`
	LastGoodAt time.Time       `json:"lastGoodAt,omitzero"`
	Windows    []ServiceWindow `json:"windows,omitempty"`
	Groups     []ServiceGroup  `json:"groups,omitempty"`
	Details    []ServiceDetail `json:"details,omitempty"`
}

// ServiceEvent is a child-to-parent line: ready (agents), snapshot (readings),
// bye, or error (message).
type ServiceEvent struct {
	V        int            `json:"v"`
	Op       string         `json:"op"`
	Message  string         `json:"message,omitempty"`
	Agents   []string       `json:"agents,omitempty"`
	Readings []ServiceAgent `json:"readings,omitempty"`
}

// DecodeRequest reads one parent line. A wrong version, an unknown op, or a
// refresh id that is neither "all" nor an agent is an error; the service loop
// turns it into an error event and keeps running.
func DecodeRequest(line []byte) (ServiceRequest, error) {
	var req ServiceRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return ServiceRequest{}, err
	}
	if req.V != ServiceVersion {
		return ServiceRequest{}, fmt.Errorf("service protocol: unsupported version %d", req.V)
	}
	switch req.Op {
	case "hello", "shutdown":
	case "refresh":
		if req.ID == "" {
			return ServiceRequest{}, fmt.Errorf("service protocol: refresh needs an agent id or \"all\"")
		}
		if req.ID != "all" && !slices.Contains(ItemAll[:], req.ID) {
			return ServiceRequest{}, fmt.Errorf("service protocol: refresh of unknown agent %q", req.ID)
		}
	default:
		return ServiceRequest{}, fmt.Errorf("service protocol: unknown op %q", req.Op)
	}
	return req, nil
}

// EncodeEvent marshals one child line as compact JSON. Times are normalised to
// UTC so every instant leaves as the RFC3339Nano UTC the protocol promises,
// whatever location a reader kept.
func EncodeEvent(e ServiceEvent) ([]byte, error) {
	readings := make([]ServiceAgent, len(e.Readings))
	for i, r := range e.Readings {
		r.Reset5h = r.Reset5h.UTC()
		r.ObservedAt = r.ObservedAt.UTC()
		r.LastGoodAt = r.LastGoodAt.UTC()
		if len(r.Details) > 0 {
			details := make([]ServiceDetail, len(r.Details))
			for j, d := range r.Details {
				d.ResetAt = d.ResetAt.UTC()
				details[j] = d
			}
			r.Details = details
		}
		readings[i] = r
	}
	e.Readings = readings
	return json.Marshal(e)
}
