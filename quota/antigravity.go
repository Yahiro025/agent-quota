package quota

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-quota/internal/proc"
)

const (
	antigravityMinVersion = "1.1.11"
	antigravityOutputMax  = 512 << 10
)

var antigravityUnsupported struct {
	sync.Mutex
	latched bool
}

// AntigravityPresent cheaply checks the normal command path and known Windows installs.
func AntigravityPresent() bool { return antigravityExe() != "" }

func antigravityExe() string {
	if p := proc.Find("agy"); p != "" {
		return p
	}
	paths := []string{}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		paths = append(paths, filepath.Join(local, "agy", "bin", "agy.exe"))
	}
	if home := proc.Home(); home != "" {
		paths = append(paths, filepath.Join(home, ".local", "bin", "agy.exe"))
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// Antigravity reads quota only through agy's own slash command, which owns its keyring
// credentials and token refresh. The unsupported latch prevents a prompt fallback from
// spending another model turn on every poll.
func Antigravity() Reading {
	antigravityUnsupported.Lock()
	unsupported := antigravityUnsupported.latched
	antigravityUnsupported.Unlock()
	if unsupported {
		return Fail("Antigravity usage isn't available. `agy /usage` started a model turn, so quota checks stopped for this run. Update agy and restart agent-quota.")
	}
	exe := antigravityExe()
	if exe == "" {
		return Fail("Antigravity CLI `agy` wasn't found.")
	}
	version := runAntigravity(exe, []string{"--version"}, 5*time.Second)
	if version.timedOut {
		return Fail("Antigravity CLI version check timed out.")
	}
	if version.err != nil || version.tooLarge || version.code != 0 {
		return Fail("Antigravity CLI version couldn't be read.")
	}
	got := antigravityVersion(version.stdout, version.stderr)
	if got == "" {
		return Fail("Antigravity CLI version couldn't be read.")
	}
	if !antigravityVersionAtLeast(got, antigravityMinVersion) {
		return Fail("Antigravity quota needs agy 1.1.11 or newer. Update agy to show usage safely.")
	}

	result := runAntigravity(exe, []string{"-p", "/usage", "--output-format", "json", "--print-timeout", "20s"}, 25*time.Second)
	if result.timedOut {
		return Fail("Antigravity CLI didn't answer the usage check in time.")
	}
	if result.tooLarge {
		return Fail("Antigravity CLI returned too much output to read safely.")
	}
	if result.err != nil {
		return Fail("Antigravity CLI couldn't be started.")
	}
	reading, modelTurn := parseAntigravityOutput(result.stdout, time.Now())
	if modelTurn {
		antigravityUnsupported.Lock()
		antigravityUnsupported.latched = true
		antigravityUnsupported.Unlock()
		return Fail("Antigravity usage isn't available. `agy /usage` started a model turn, so quota checks stopped for this run. Update agy and restart agent-quota.")
	}
	if reading.OK() {
		return reading
	}
	diagnostic := result.stdout + "\n" + result.stderr
	if antigravitySignedOut(strings.ToLower(diagnostic)) || antigravityStructuredAuth(diagnostic) {
		return Fail("Antigravity isn't signed in. Open agy to renew its sign-in, then refresh the quota.")
	}
	if result.code != 0 {
		return Fail(fmt.Sprintf("Antigravity usage check failed (exit %d).", result.code))
	}
	return Fail("Antigravity CLI didn't report a readable quota.")
}

// ParseAntigravity parses agy's print-mode JSON envelope without accessing the CLI or disk.
func ParseAntigravity(text string, now time.Time) Reading {
	reading, modelTurn := parseAntigravityOutput(text, now)
	if modelTurn {
		return Fail("Antigravity answered `/usage` as a model prompt; no quota was accepted.")
	}
	return reading
}

type antigravityBucket struct {
	group  string
	window string
	used   float64
	reset  *time.Time
}

func parseAntigravityOutput(output string, now time.Time) (Reading, bool) {
	var found Reading
	modelTurn := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var envelope map[string]any
		if json.Unmarshal([]byte(line), &envelope) != nil {
			continue
		}
		if turns, ok := envelope["num_turns"].(float64); ok && turns != 0 {
			modelTurn = true
		}
		if conversation, ok := envelope["conversation_id"].(string); ok && strings.TrimSpace(conversation) != "" {
			modelTurn = true
		}
		if envelope["status"] != "SUCCESS" {
			continue
		}
		command, ok := envelope["command"].(map[string]any)
		if !ok || command["name"] != "usage" {
			continue
		}
		data, ok := command["data"].(map[string]any)
		if !ok {
			continue
		}
		groups, ok := data["groups"].([]any)
		if !ok {
			continue
		}
		buckets := parseAntigravityGroups(groups)
		if len(buckets) == 0 {
			continue
		}
		found = antigravityReading(buckets, now)
	}
	if modelTurn {
		return Fail("Antigravity answered `/usage` as a model prompt; no quota was accepted."), true
	}
	if found.OK() {
		return found, false
	}
	return Fail("Antigravity CLI didn't report a readable quota."), false
}

func parseAntigravityGroups(groups []any) []antigravityBucket {
	var out []antigravityBucket
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			continue
		}
		name, ok := group["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		rawBuckets, ok := group["buckets"].([]any)
		if !ok {
			continue
		}
		for _, raw := range rawBuckets {
			bucket, ok := raw.(map[string]any)
			if !ok || bucket["disabled"] == true {
				continue
			}
			if id, ok := bucket["id"].(string); !ok || strings.TrimSpace(id) == "" {
				continue
			}
			fraction, ok := bucket["remaining_fraction"].(float64)
			if !ok || math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction < 0 || fraction > 1 {
				continue
			}
			window, _ := bucket["window"].(string)
			var reset *time.Time
			if value, ok := bucket["reset_time"].(string); ok {
				if parsed, valid := ParseTime(value); valid {
					reset = &parsed
				}
			}
			out = append(out, antigravityBucket{group: strings.TrimSpace(name), window: window, used: (1 - fraction) * 100, reset: reset})
		}
	}
	return out
}

func antigravityReading(buckets []antigravityBucket, now time.Time) Reading {
	maxUsed := 0.0
	bottleneck := -1
	parts := make([]string, 0, len(buckets))
	for i, bucket := range buckets {
		if bucket.used > maxUsed || bottleneck < 0 {
			maxUsed = bucket.used
			bottleneck = i
		}
		window := bucket.window
		if window == "" {
			window = "usage"
		}
		parts = append(parts, fmt.Sprintf("%s %s %s%%", bucket.group, window, Custom(bucket.used, 0)))
	}
	if bottleneck >= 0 && buckets[bottleneck].reset != nil {
		reset := *buckets[bottleneck].reset
		shown := FormatLocal(reset, "d MMM HH:mm")
		if sameLocalDay(reset, now) {
			shown = FormatLocal(reset, "HH:mm")
		}
		parts = append(parts, "resets "+shown)
	}
	return newReading(maxUsed, strings.Join(parts, " · "))
}

func antigravityStructuredAuth(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		at := strings.Index(line, "AGY_ERROR:")
		if at < 0 {
			continue
		}
		var value map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(line[at+len("AGY_ERROR:"):])), &value) != nil {
			continue
		}
		status, _ := value["status"].(string)
		status = strings.ToUpper(status)
		code, _ := value["error_code"].(float64)
		if status == "UNAUTHENTICATED" || status == "AUTHENTICATION_REQUIRED" || code == 401 {
			return true
		}
	}
	return false
}

func antigravitySignedOut(output string) bool {
	for _, phrase := range []string{"not logged in", "not signed in", "not authenticated", "unauthenticated", "run agy login", "please sign in", "please log in", "no credentials", "authentication required"} {
		if strings.Contains(output, phrase) {
			return true
		}
	}
	return false
}

func antigravityVersion(stdout, stderr string) string {
	for _, token := range strings.Fields(strings.TrimSpace(stdout + " " + stderr)) {
		token = strings.Trim(token, "vV,;()[]")
		if _, ok := antigravityVersionParts(token); ok {
			return token
		}
	}
	return ""
}

func antigravityVersionParts(version string) ([3]int, bool) {
	var out [3]int
	if strings.ContainsAny(version, "-+") {
		return out, false
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return out, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func antigravityVersionAtLeast(got, min string) bool {
	a, aok := antigravityVersionParts(got)
	b, bok := antigravityVersionParts(min)
	if !aok || !bok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

type antigravityResult struct {
	stdout, stderr string
	code           int
	timedOut       bool
	tooLarge       bool
	err            error
}

type antigravityCapture struct {
	mu       sync.Mutex
	stdout   strings.Builder
	stderr   strings.Builder
	total    int
	tooLarge bool
	group    *proc.Group
}

func (c *antigravityCapture) read(pipe *os.File, stdout bool, done chan<- struct{}) {
	defer func() { pipe.Close(); done <- struct{}{} }()
	buf := make([]byte, 32<<10)
	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			c.mu.Lock()
			remaining := antigravityOutputMax - c.total
			keep := n
			if keep > remaining {
				keep = remaining
				if !c.tooLarge {
					c.tooLarge = true
					go c.group.Kill()
				}
			}
			if keep > 0 {
				if stdout {
					c.stdout.Write(buf[:keep])
				} else {
					c.stderr.Write(buf[:keep])
				}
				c.total += keep
			}
			c.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func runAntigravity(exe string, args []string, limit time.Duration) antigravityResult {
	group, err := proc.Spawn(proc.Hidden(exe, args...))
	if err != nil {
		return antigravityResult{err: err}
	}
	stdin, stdout, stderr := group.TakePipes()
	if stdin != nil {
		stdin.Close()
	}
	capture := &antigravityCapture{group: group}
	done := make(chan struct{}, 2)
	go capture.read(stdout, true, done)
	go capture.read(stderr, false, done)
	code, exited := group.WaitTimeout(limit)
	var closeOnce sync.Once
	closeGroup := func() { closeOnce.Do(group.Close) }
	defer closeGroup()
	if !exited {
		group.Kill()
		code, _ = group.WaitTimeout(2 * time.Second)
	}
	if !exited {
		closeGroup()
	}
	for range 2 {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			closeGroup()
		}
	}
	capture.mu.Lock()
	result := antigravityResult{
		stdout:   capture.stdout.String(),
		stderr:   capture.stderr.String(),
		code:     code,
		timedOut: !exited,
		tooLarge: capture.tooLarge,
	}
	capture.mu.Unlock()
	return result
}
