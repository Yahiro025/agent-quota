package quota

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"agent-quota/internal/core"
)

const (
	claudeSnapshotMaxInput = 1 << 20
	claudeSnapshotMaxFiles = 128
	claudeSnapshotFresh    = 5 * time.Minute
)

var claudeSnapshotMu sync.Mutex

type claudeSnapshotWindow struct {
	Used    float64 `json:"used"`
	ResetAt int64   `json:"reset_at"`
	Present bool    `json:"present"`
}

type claudeSnapshot struct {
	ObservedAt  int64                `json:"observed_at"`
	Session     string               `json:"session"`
	Fingerprint string               `json:"fingerprint"`
	FiveHour    claudeSnapshotWindow `json:"five_hour"`
	SevenDay    claudeSnapshotWindow `json:"seven_day"`
}

type claudeStatusInput struct {
	SessionID  string `json:"session_id"`
	RateLimits struct {
		FiveHour *struct {
			UsedPercentage *float64 `json:"used_percentage"`
			ResetsAt       *int64   `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay *struct {
			UsedPercentage *float64 `json:"used_percentage"`
			ResetsAt       *int64   `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"rate_limits"`
	Cost *struct {
		TotalAPIDurationMS *json.Number `json:"total_api_duration_ms"`
	} `json:"cost"`
}

func claudeScope() string {
	p := strings.TrimSpace(ClaudeHome())
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return strings.ToLower(filepath.Clean(p))
}

func claudeHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func snapshotPath(scopeHash, sessionHash string) string {
	return filepath.Join(core.Support(), "claude-snapshots", scopeHash, sessionHash+".json")
}

// CaptureClaudeSnapshot stores only the two documented quota windows and a hash of
// their values plus Claude's actual-response duration counter. Status-line callbacks
// also fire for timer redraws, so an identical observation must keep its original time.
func CaptureClaudeSnapshot(input []byte, now time.Time) bool {
	if len(input) == 0 || len(input) > claudeSnapshotMaxInput || now.IsZero() {
		return false
	}
	scope := claudeScope()
	if scope == "" {
		return false
	}
	var in claudeStatusInput
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.UseNumber()
	if dec.Decode(&in) != nil || strings.TrimSpace(in.SessionID) == "" {
		return false
	}
	if dec.Decode(new(any)) != io.EOF {
		return false
	}
	now = now.UTC()
	var snap claudeSnapshot
	snap.ObservedAt = now.UnixNano()
	snap.Session = claudeHash(in.SessionID)
	snap.FiveHour = snapshotWindow(in.RateLimits.FiveHour, now)
	snap.SevenDay = snapshotWindow(in.RateLimits.SevenDay, now)
	if !snap.FiveHour.Present && !snap.SevenDay.Present {
		return false
	}
	material := struct {
		FiveHour claudeSnapshotWindow `json:"five_hour"`
		SevenDay claudeSnapshotWindow `json:"seven_day"`
		Duration string               `json:"duration"`
	}{snap.FiveHour, snap.SevenDay, ""}
	if in.Cost != nil && in.Cost.TotalAPIDurationMS != nil {
		material.Duration = in.Cost.TotalAPIDurationMS.String()
	}
	b, err := json.Marshal(material)
	if err != nil {
		return false
	}
	snap.Fingerprint = claudeHash(string(b))

	claudeSnapshotMu.Lock()
	defer claudeSnapshotMu.Unlock()
	dir := filepath.Dir(snapshotPath(claudeHash(scope), snap.Session))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	path := snapshotPath(claudeHash(scope), snap.Session)
	if old, err := readClaudeSnapshot(path); err == nil && old.Fingerprint == snap.Fingerprint {
		return false
	}
	_, statErr := os.Lstat(path)
	newSessionFile := os.IsNotExist(statErr)
	b, err = json.Marshal(snap)
	if err != nil {
		return false
	}
	tmp, err := os.CreateTemp(dir, ".capture-*.tmp")
	if err != nil {
		return false
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(tmpName, path) != nil {
		return false
	}
	if newSessionFile {
		pruneClaudeSnapshots(claudeHash(scope))
	}
	return true
}

type claudeSnapshotFile struct {
	name string
	path string
	at   int64
	mod  time.Time
}

func strictClaudeSnapshotName(name string) bool {
	if len(name) != 69 || !strings.HasSuffix(name, ".json") {
		return false
	}
	for _, c := range name[:64] {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// pruneClaudeSnapshots bounds the app-owned per-session cache. It only removes regular
// files with the exact hashed filename form under the resolved scope directory.
func pruneClaudeSnapshots(scopeHash string) {
	if !strictClaudeSnapshotName(scopeHash + ".json") {
		return
	}
	support, err := filepath.Abs(core.Support())
	if err != nil {
		return
	}
	cacheRoot := filepath.Join(support, "claude-snapshots")
	dir := filepath.Join(cacheRoot, scopeHash)
	resolved, err := filepath.Abs(dir)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(dir) || filepath.Dir(resolved) != filepath.Clean(cacheRoot) {
		return
	}
	for _, path := range []string{support, cacheRoot, resolved} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return
		}
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return
	}
	files := make([]claudeSnapshotFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strictClaudeSnapshotName(name) {
			continue
		}
		path := filepath.Join(resolved, name)
		if filepath.Dir(filepath.Clean(path)) != filepath.Clean(resolved) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		at := info.ModTime().UnixNano()
		if snapshot, err := readClaudeSnapshot(path); err == nil {
			at = snapshot.ObservedAt
		}
		files = append(files, claudeSnapshotFile{name: name, path: path, at: at, mod: info.ModTime()})
	}
	if excess := len(files) - claudeSnapshotMaxFiles; excess > 0 {
		sort.Slice(files, func(i, j int) bool {
			if files[i].at != files[j].at {
				return files[i].at < files[j].at
			}
			if !files[i].mod.Equal(files[j].mod) {
				return files[i].mod.Before(files[j].mod)
			}
			return files[i].name < files[j].name
		})
		for _, file := range files[:excess] {
			info, err := os.Lstat(file.path)
			if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && filepath.Dir(filepath.Clean(file.path)) == filepath.Clean(resolved) {
				_ = os.Remove(file.path)
			}
		}
	}
}

func snapshotWindow(in *struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}, now time.Time) claudeSnapshotWindow {
	if in == nil || in.UsedPercentage == nil || in.ResetsAt == nil {
		return claudeSnapshotWindow{}
	}
	used, reset := *in.UsedPercentage, *in.ResetsAt
	window := claudeSnapshotWindow{Used: used, ResetAt: reset, Present: true}
	if !validClaudeSnapshotWindow(window, now) {
		return claudeSnapshotWindow{}
	}
	return window
}

func validClaudeSnapshotWindow(w claudeSnapshotWindow, now time.Time) bool {
	if !w.Present || math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 100 {
		return false
	}
	reset := time.Unix(w.ResetAt, 0)
	return w.ResetAt > now.Unix() && !reset.After(now.Add(30*24*time.Hour))
}

func readClaudeSnapshot(path string) (claudeSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return claudeSnapshot{}, err
	}
	defer f.Close()
	var s claudeSnapshot
	dec := json.NewDecoder(io.LimitReader(f, 4096))
	err = dec.Decode(&s)
	if err != nil || s.ObservedAt <= 0 || s.Fingerprint == "" {
		return claudeSnapshot{}, fmt.Errorf("invalid Claude snapshot")
	}
	return s, nil
}

func claudeSnapshotReading(now time.Time) (Reading, bool) {
	scope := claudeScope()
	if scope == "" {
		return Reading{}, false
	}
	claudeSnapshotMu.Lock()
	defer claudeSnapshotMu.Unlock()
	dir := filepath.Join(core.Support(), "claude-snapshots", claudeHash(scope))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Reading{}, false
	}
	sort.Slice(entries, func(i, j int) bool {
		a, _ := entries[i].Info()
		b, _ := entries[j].Info()
		if a == nil || b == nil {
			return entries[i].Name() > entries[j].Name()
		}
		return a.ModTime().After(b.ModTime())
	})
	var newest *claudeSnapshot
	for _, entry := range entries[:min(len(entries), claudeSnapshotMaxFiles)] {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		s, err := readClaudeSnapshot(filepath.Join(dir, entry.Name()))
		if err != nil || s.Session != strings.TrimSuffix(entry.Name(), ".json") {
			continue
		}
		if !validClaudeSnapshotWindow(s.FiveHour, now) {
			s.FiveHour = claudeSnapshotWindow{}
		}
		if !validClaudeSnapshotWindow(s.SevenDay, now) {
			s.SevenDay = claudeSnapshotWindow{}
		}
		if !s.FiveHour.Present && !s.SevenDay.Present {
			continue
		}
		at := time.Unix(0, s.ObservedAt)
		if at.After(now.Add(30*time.Second)) || now.Sub(at) > claudeSnapshotFresh {
			continue
		}
		if newest == nil || s.ObservedAt > newest.ObservedAt {
			copy := s
			newest = &copy
		}
	}
	if newest == nil {
		return Reading{}, false
	}
	var wins []struct {
		used  float64
		label string
		reset int64
	}
	if newest.FiveHour.Present {
		wins = append(wins, struct {
			used  float64
			label string
			reset int64
		}{newest.FiveHour.Used, "5h", newest.FiveHour.ResetAt})
	}
	if newest.SevenDay.Present {
		wins = append(wins, struct {
			used  float64
			label string
			reset int64
		}{newest.SevenDay.Used, "week", newest.SevenDay.ResetAt})
	}
	if len(wins) == 0 {
		return Reading{}, false
	}
	top := wins[0]
	parts := make([]string, 0, len(wins))
	for _, w := range wins {
		parts = append(parts, fmt.Sprintf("%s %s%%", w.label, Custom(w.used, 0)))
		if w.used > top.used {
			top = w
		}
	}
	resetAt := time.Unix(top.reset, 0)
	shown := FormatLocal(resetAt, "d MMM HH:mm")
	if sameLocalDay(resetAt, now) {
		shown = FormatLocal(resetAt, "HH:mm")
	}
	observedAt := time.Unix(0, newest.ObservedAt)
	r := newReading(top.used, strings.Join(parts, " · ")+" · as of "+FormatLocal(observedAt, "HH:mm")+" · resets "+shown)
	// Detail's reset is the top window's, which can be the week's; Reset5h always
	// carries the five-hour instant so the pill counts down the right window.
	if newest.FiveHour.Present {
		if t := time.Unix(newest.FiveHour.ResetAt, 0).UTC(); t.After(now) {
			r.Reset5h = t
		}
	}
	return r, true
}
