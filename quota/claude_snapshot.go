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
	"agent-quota/internal/proc"
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
	r.ObservedAt = observedAt
	for _, w := range wins {
		r.Windows = append(r.Windows, Window{Label: w.label, Used: w.used})
		reset := time.Unix(w.reset, 0).UTC()
		r.Details = append(r.Details, percentDetail(w.label, w.used, &reset))
	}
	// Detail's reset is the top window's, which can be the week's; Reset5h always
	// carries the five-hour instant so the pill counts down the right window.
	if newest.FiveHour.Present {
		if t := time.Unix(newest.FiveHour.ResetAt, 0).UTC(); t.After(now) {
			r.Reset5h = t
		}
	}
	return r, true
}

// recordClaudeReadingSnapshot persists a successful OAuth usage reading into the
// scoped snapshot directory so subsequent checks (across restarts, rate limits,
// or token expirations) have the latest reading on disk.
func recordClaudeReadingSnapshot(r Reading, now time.Time) bool {
	if !r.OK() || now.IsZero() {
		return false
	}
	scope := claudeScope()
	if scope == "" {
		return false
	}
	now = now.UTC()
	var snap claudeSnapshot
	snap.ObservedAt = now.UnixNano()
	snap.Session = claudeHash("oauth-usage")
	for i, w := range r.Windows {
		var resetUnix int64
		if i < len(r.Details) && !r.Details[i].ResetAt.IsZero() {
			resetUnix = r.Details[i].ResetAt.UTC().Unix()
		}
		cw := claudeSnapshotWindow{Used: w.Used, ResetAt: resetUnix, Present: true}
		switch strings.ToLower(w.Label) {
		case "5h":
			snap.FiveHour = cw
		case "week":
			snap.SevenDay = cw
		}
	}
	if !snap.FiveHour.Present && !snap.SevenDay.Present {
		return false
	}
	material := struct {
		FiveHour claudeSnapshotWindow `json:"five_hour"`
		SevenDay claudeSnapshotWindow `json:"seven_day"`
	}{snap.FiveHour, snap.SevenDay}
	b, err := json.Marshal(material)
	if err != nil {
		return false
	}
	snap.Fingerprint = claudeHash(string(b))

	claudeSnapshotMu.Lock()
	defer claudeSnapshotMu.Unlock()
	scopeHash := claudeHash(scope)
	path := snapshotPath(scopeHash, snap.Session)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
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
		pruneClaudeSnapshots(scopeHash)
	}
	return true
}

func fallbackClaudeSnapshotWindow(w claudeSnapshotWindow, now time.Time) claudeSnapshotWindow {
	if !w.Present || math.IsNaN(w.Used) || math.IsInf(w.Used, 0) || w.Used < 0 || w.Used > 100 || w.ResetAt < 0 {
		return claudeSnapshotWindow{}
	}
	if w.ResetAt > 0 {
		reset := time.Unix(w.ResetAt, 0)
		if reset.After(now.Add(30 * 24 * time.Hour)) {
			return claudeSnapshotWindow{}
		}
		if w.ResetAt <= now.Unix() {
			w.Used = 0
			w.ResetAt = 0
		}
	}
	return w
}

// claudeCachedReading returns the freshest local Claude reading when live OAuth
// fails (due to rate limiting, expired short-lived token, or cleared credentials),
// comparing persisted status-line/OAuth snapshots against Claude Code's own
// cachedUsageUtilization in .claude.json (and its backups).
func claudeCachedReading(now time.Time) (Reading, bool) {
	snapR, snapOK := claudeFallbackSnapshotReading(now)
	cfgR, cfgOK := claudeConfigCachedReading(now)
	switch {
	case snapOK && cfgOK:
		if cfgR.ObservedAt.After(snapR.ObservedAt) {
			return cfgR, true
		}
		return snapR, true
	case cfgOK:
		return cfgR, true
	case snapOK:
		return snapR, true
	default:
		return Reading{}, false
	}
}

func claudeFallbackSnapshotReading(now time.Time) (Reading, bool) {
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
		s.FiveHour = fallbackClaudeSnapshotWindow(s.FiveHour, now)
		s.SevenDay = fallbackClaudeSnapshotWindow(s.SevenDay, now)
		if !s.FiveHour.Present && !s.SevenDay.Present {
			continue
		}
		at := time.Unix(0, s.ObservedAt).UTC()
		if at.After(now.Add(30 * time.Second)) {
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
		if w.used > top.used || (w.used == top.used && top.reset == 0 && w.reset > 0) {
			top = w
		}
	}
	observedAt := time.Unix(0, newest.ObservedAt).UTC()
	plan := claudeLocalPlan()
	planText := ""
	if plan != "" {
		planText = capital(plan) + " · "
	}
	resetText := ""
	if top.reset > 0 {
		resetAt := time.Unix(top.reset, 0)
		shown := FormatLocal(resetAt, "d MMM HH:mm")
		if sameLocalDay(resetAt, now) {
			shown = FormatLocal(resetAt, "HH:mm")
		}
		resetText = " · resets " + shown
	}
	r := newReading(top.used, planText+strings.Join(parts, " · ")+" · as of "+FormatLocal(observedAt, "HH:mm")+resetText)
	r.ObservedAt = observedAt
	r.Plan = strings.TrimSuffix(planText, " · ")
	for _, w := range wins {
		r.Windows = append(r.Windows, Window{Label: w.label, Used: w.used})
		var resetPtr *time.Time
		if w.reset > 0 {
			reset := time.Unix(w.reset, 0).UTC()
			resetPtr = &reset
		}
		r.Details = append(r.Details, percentDetail(w.label, w.used, resetPtr))
	}
	if newest.FiveHour.Present && newest.FiveHour.ResetAt > 0 {
		if t := time.Unix(newest.FiveHour.ResetAt, 0).UTC(); t.After(now) {
			r.Reset5h = t
		}
	}
	return r, true
}

type claudeConfigCache struct {
	OAuthAccount *struct {
		AccountUUID      string `json:"accountUuid"`
		OrganizationType string `json:"organizationType"`
	} `json:"oauthAccount"`
	CachedUsageUtilization *struct {
		FetchedAtMs int64           `json:"fetchedAtMs"`
		AccountUUID string          `json:"accountUuid"`
		Utilization json.RawMessage `json:"utilization"`
	} `json:"cachedUsageUtilization"`
}

func normalizeClaudeOrgType(orgType string) string {
	s := strings.TrimSpace(strings.ToLower(orgType))
	s = strings.TrimPrefix(s, "claude_")
	switch s {
	case "pro", "max", "team", "enterprise":
		return s
	}
	return ""
}

func claudeCredentialsPlan(home string) string {
	if home == "" {
		return ""
	}
	b, err := core.ReadFile(filepath.Join(home, ".credentials.json"))
	if err != nil {
		return ""
	}
	sign := ClaudeSignIn(core.TextOf(b))
	if sign.Plan != nil && strings.TrimSpace(*sign.Plan) != "" {
		return strings.TrimSpace(*sign.Plan)
	}
	return ""
}

func claudeConfigPaths() []string {
	home := strings.TrimSpace(ClaudeHome())
	if home == "" {
		return nil
	}
	var paths []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}
	// When CLAUDE_CONFIG_DIR is not explicitly set, Claude Code writes its main
	// config to ~/.claude.json and backups under ~/.claude/backups/.
	if envDir("CLAUDE_CONFIG_DIR") == "" {
		if userHome := strings.TrimSpace(proc.Home()); userHome != "" {
			add(filepath.Join(userHome, ".claude.json"))
		}
	}
	add(filepath.Join(home, ".claude.json"))
	add(filepath.Join(home, ".config.json"))
	backupDir := filepath.Join(home, "backups")
	if entries, err := os.ReadDir(backupDir); err == nil {
		var backups []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), ".claude.json.backup.") {
				backups = append(backups, filepath.Join(backupDir, e.Name()))
			}
		}
		sort.Slice(backups, func(i, j int) bool { return backups[i] > backups[j] })
		for _, p := range backups[:min(len(backups), 8)] {
			add(p)
		}
	}
	return paths
}

func claudeLocalPlan() string {
	home := strings.TrimSpace(ClaudeHome())
	if p := claudeCredentialsPlan(home); p != "" {
		return p
	}
	for _, p := range claudeConfigPaths() {
		b, err := core.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg claudeConfigCache
		if json.Unmarshal(b, &cfg) == nil && cfg.OAuthAccount != nil {
			if plan := normalizeClaudeOrgType(cfg.OAuthAccount.OrganizationType); plan != "" {
				return plan
			}
		}
	}
	return ""
}

func claudeConfigCachedReading(now time.Time) (Reading, bool) {
	paths := claudeConfigPaths()
	if len(paths) == 0 {
		return Reading{}, false
	}
	credPlan := claudeCredentialsPlan(strings.TrimSpace(ClaudeHome()))
	var best Reading
	var hasBest bool
	for _, p := range paths {
		b, err := core.ReadFile(p)
		if err != nil {
			continue
		}
		var cfg claudeConfigCache
		if err := json.Unmarshal(b, &cfg); err != nil || cfg.CachedUsageUtilization == nil {
			continue
		}
		cu := cfg.CachedUsageUtilization
		if cu.FetchedAtMs <= 0 || len(cu.Utilization) == 0 {
			continue
		}
		if cfg.OAuthAccount != nil && cfg.OAuthAccount.AccountUUID != "" && cu.AccountUUID != "" && cfg.OAuthAccount.AccountUUID != cu.AccountUUID {
			continue
		}
		observedAt, ok := fromUnixMs(float64(cu.FetchedAtMs))
		if !ok || observedAt.After(now.Add(30*time.Second)) {
			continue
		}
		plan := credPlan
		if plan == "" && cfg.OAuthAccount != nil {
			plan = normalizeClaudeOrgType(cfg.OAuthAccount.OrganizationType)
		}
		var planPtr *string
		if plan != "" {
			planPtr = &plan
		}
		r := ParseClaudeUsage(string(cu.Utilization), planPtr, now)
		if !r.OK() {
			continue
		}
		r.ObservedAt = observedAt
		if !hasBest || r.ObservedAt.After(best.ObservedAt) {
			best = r
			hasBest = true
		}
	}
	return best, hasBest
}
