package quota

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"agent-quota/internal/core"
	"agent-quota/internal/proc"
)

// Devin credentials: %APPDATA%\devin\credentials.toml with windsurf_api_key + api_server_url.
// Two string values parsed by hand; no TOML dependency.
type devinCreds struct {
	key    string
	server string
}

func devinCredFile() string {
	base := core.AppData()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "devin", "credentials.toml")
}

func parseDevinCreds(text string) (devinCreds, bool) {
	var c devinCreds
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		switch k {
		case "windsurf_api_key":
			c.key = v
		case "api_server_url":
			c.server = strings.TrimRight(v, "/")
		}
	}
	if c.key == "" || c.server == "" {
		return devinCreds{}, false
	}
	return c, true
}

// DevinPresent: credentials file exists.
func DevinPresent() bool {
	f := devinCredFile()
	if f == "" {
		return false
	}
	st, err := os.Stat(f)
	return err == nil && st.Mode().IsRegular()
}

// ParseDevin reads GetUserStatus: plan + daily/weekly remaining percents + reset unix times.
// Ring = max(100-daily, 100-weekly). Detail: "Pro · day 62% · week 31% · resets 00:00".
func ParseDevin(text string, now time.Time) Reading {
	root, ok := parse(text)
	if !ok {
		return Fail("Devin's answer couldn't be read.")
	}
	// Devin wraps the subscription fields in userStatus.planStatus. Keep the older
	// flat shape accepted for existing fixtures and any server rollout overlap.
	usage := root
	nested := false
	if status, ok := objOf(root, "userStatus"); ok {
		if planStatus, ok := objOf(status, "planStatus"); ok {
			usage = planStatus
			nested = true
		} else {
			return Fail("Devin didn't report plan usage.")
		}
	}
	plan := ""
	if info, ok := objOf(usage, "planInfo"); ok {
		if p := strOf(info, "planName"); p != nil && *p != "" {
			plan = capital(*p)
		}
	}
	if plan == "" {
		if p := strOf(usage, "plan"); p != nil && *p != "" {
			plan = capital(*p)
		} else if p := strOf(usage, "planName"); p != nil && *p != "" {
			plan = capital(*p)
		}
	}
	if plan == "" && nested {
		if p := strOf(root, "plan"); p != nil && *p != "" {
			plan = capital(*p)
		}
	}
	daily := numOf(usage, "dailyQuotaRemainingPercent")
	weekly := numOf(usage, "weeklyQuotaRemainingPercent")
	if daily == nil && weekly == nil {
		// Nested shapes: quota.daily.remainingPercent.
		if q, ok := usage.Get("quota"); ok {
			if d, ok := q.Get("daily"); ok {
				daily = numOf(d, "remainingPercent")
				if daily == nil {
					daily = numOf(d, "remaining")
				}
			}
			if w, ok := q.Get("weekly"); ok {
				weekly = numOf(w, "remainingPercent")
				if weekly == nil {
					weekly = numOf(w, "remaining")
				}
			}
		}
	}
	if daily == nil && weekly == nil {
		return Fail("Devin didn't report plan usage.")
	}
	used := 0.0
	parts := []string{}
	if daily != nil {
		u := 100 - *daily
		used = max(used, min(max(u, 0), 100))
		parts = append(parts, fmt.Sprintf("day %s%%", Custom(min(max(u, 0), 100), 0)))
	}
	if weekly != nil {
		u := 100 - *weekly
		used = max(used, min(max(u, 0), 100))
		parts = append(parts, fmt.Sprintf("week %s%%", Custom(min(max(u, 0), 100), 0)))
	}
	resetText := ""
	for _, name := range []string{"dailyQuotaResetAtUnix", "weeklyQuotaResetAtUnix", "resetAt", "dailyResetAt", "reset_time", "resetsAt"} {
		var seconds *float64
		if name == "dailyQuotaResetAtUnix" || name == "weeklyQuotaResetAtUnix" {
			if raw := strOf(usage, name); raw != nil {
				if parsed, err := strconv.ParseFloat(*raw, 64); err == nil {
					seconds = &parsed
				}
			}
		} else {
			seconds = numOf(usage, name)
		}
		if e := seconds; e != nil {
			if t, ok := fromUnixSecs(*e); ok {
				resetText = " · resets " + FormatLocal(t, "HH:mm")
				break
			}
		}
	}
	detail := strings.Join(parts, " · ") + resetText
	if plan != "" {
		detail = plan + " · " + detail
	}
	return newReading(used, detail)
}

// Devin posts GetUserStatus; ideVersion comes from `devin version` once at startup.
func Devin() Reading {
	f := devinCredFile()
	if f == "" {
		return Fail("Devin isn't installed.")
	}
	b, err := core.ReadFile(f)
	if err != nil {
		return Fail("Devin isn't signed in.")
	}
	creds, ok := parseDevinCreds(core.TextOf(b))
	if !ok {
		return Fail("Devin isn't signed in.")
	}
	return DevinWith(creds.server, creds.key, devinVersion(), time.Now().UTC())
}

// DevinWith posts to a server (httptest in tests).
func DevinWith(server, key, ideVersion string, now time.Time) Reading {
	if !validDevinVersion(ideVersion) {
		return Fail("Devin CLI version couldn't be read.")
	}
	url := server + "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	metadata := struct {
		APIKey           string `json:"apiKey"`
		IDEName          string `json:"ideName"`
		IDEType          string `json:"ideType"`
		IDEVersion       string `json:"ideVersion"`
		ExtensionName    string `json:"extensionName"`
		ExtensionVersion string `json:"extensionVersion"`
		Locale           string `json:"locale"`
		OS               string `json:"os"`
		UserJWT          string `json:"userJwt"`
	}{key, "devin-cli", "chisel", ideVersion, "chisel", ideVersion, "en", "windows", ""}
	bodyBytes, err := json.Marshal(struct {
		Metadata any `json:"metadata"`
	}{metadata})
	if err != nil {
		return Fail("Couldn't reach Devin's server.")
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return Fail("Couldn't reach Devin's server.")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Fail("Couldn't reach Devin's server.")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return Fail("Devin's answer couldn't be read.")
	}
	switch res.StatusCode {
	case 200:
		return ParseDevin(core.Lossy(b), now)
	case 401, 403:
		return Fail("Devin's sign-in was refused — sign in again.")
	case 429:
		return Fail("Devin is limiting usage checks; trying again in five minutes.")
	case 500:
		return Fail("Devin's server failed — trying again in five minutes.")
	default:
		return Fail(fmt.Sprintf("Devin's server answered %d.", res.StatusCode))
	}
}

var devinVerOnce = struct {
	v   string
	ok  bool
	set bool
}{}

func devinVersion() string {
	if devinVerOnce.set {
		return devinVerOnce.v
	}
	devinVerOnce.set = true
	exe := proc.Find("devin")
	if exe == "" {
		return ""
	}
	text, failed := devinCmd(exe, 10*time.Second)
	if failed != nil {
		return ""
	}
	version, ok := parseDevinVersion(text)
	if !ok {
		return ""
	}
	devinVerOnce.v = version
	devinVerOnce.ok = true
	return devinVerOnce.v
}

var devinSemver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

func parseDevinVersion(output string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 2 || fields[0] != "devin" || !devinSemver.MatchString(fields[1]) {
		return "", false
	}
	return fields[1], true
}

func validDevinVersion(version string) bool {
	return devinSemver.MatchString(version)
}

func devinCmd(exe string, limit time.Duration) (string, *Reading) {
	g, err := proc.Spawn(proc.Hidden(exe, "version"))
	if err != nil {
		r := Fail(fmt.Sprintf("devin failed: %v", err))
		return "", &r
	}
	defer g.Close()
	start := time.Now()
	stdin, stdout, stderr := g.TakePipes()
	if stdin != nil {
		stdin.Close()
	}
	type piece struct {
		i int
		s string
	}
	ch := make(chan piece, 2)
	for i, p := range [2]*os.File{stdout, stderr} {
		go func() {
			var b []byte
			if p != nil {
				b, _ = io.ReadAll(p)
				p.Close()
			}
			ch <- piece{i, core.Lossy(b)}
		}()
	}
	code, ok := g.WaitTimeout(limit)
	if !ok {
		g.Kill()
		r := Fail("devin didn't answer in time.")
		return "", &r
	}
	if code != 0 {
		r := Fail("devin version failed.")
		return "", &r
	}
	var out [2]string
	for range 2 {
		select {
		case x := <-ch:
			out[x.i] = x.s
		case <-time.After(max(limit-time.Since(start), 0)):
			g.Kill()
			r := Fail("devin didn't answer in time.")
			return "", &r
		}
	}
	return out[0] + "\n" + out[1], nil
}
