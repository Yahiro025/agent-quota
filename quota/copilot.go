package quota

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"agent-quota/internal/core"
	"agent-quota/internal/proc"
)

// ParseCopilot reads `gh api copilot_internal/user`: plan + quota_snapshots.
// Ring = max over non-unlimited snapshots of (100 - percent_remaining).
// Detail: "Individual · premium 26/200 · resets 1 Nov". All-unlimited → 0%, "Unlimited".
func ParseCopilot(text string, now time.Time) Reading {
	root, ok := parse(text)
	if !ok {
		return Fail("Copilot's answer couldn't be read.")
	}
	plan := ""
	if p := strOf(root, "plan"); p != nil && *p != "" {
		plan = capital(*p)
	}
	resetText := ""
	if r := strOf(root, "quota_reset_date"); r != nil && *r != "" {
		if t, ok := ParseTime(*r); ok {
			resetText = " · resets " + FormatLocal(t, "d MMM")
		} else if len(*r) >= 10 {
			if t, err := time.Parse("2006-01-02", (*r)[:10]); err == nil {
				resetText = " · resets " + FormatLocal(t, "d MMM")
			}
		}
	}
	snaps, ok := root.Get("quota_snapshots")
	if !ok || snaps.Kind() != core.ObjKind {
		return Fail("Copilot didn't report plan usage.")
	}
	props, err := snaps.Props()
	if err != nil {
		return Fail("Copilot didn't report plan usage.")
	}
	top := -1.0
	detailEnt := ""
	for _, p := range props {
		s := p.Val
		if u, ok := s.Get("unlimited"); ok {
			if b, err := u.Bool(); err == nil && b {
				continue
			}
		}
		rem := numOf(s, "percent_remaining")
		if rem == nil {
			continue
		}
		used := min(max(100-*rem, 0), 100)
		if used > top {
			top = used
			detailEnt = fmt.Sprintf("%s %s%%", shortEnt(p.Key), Custom(used, 0))
			if u := numOf(s, "remaining"); u != nil {
				if l := numOf(s, "limit"); l != nil && *l > 0 {
					detailEnt = fmt.Sprintf("%s %s/%s", shortEnt(p.Key), Custom(*l-*u, 0), Custom(*l, 0))
				}
			}
		}
	}
	if top < 0 {
		if plan != "" {
			return newReading(0, plan+" · Unlimited")
		}
		return newReading(0, "Unlimited")
	}
	detail := ""
	if plan != "" {
		detail = plan + " · "
	}
	_ = now
	return newReading(top, detail+detailEnt+resetText)
}

func shortEnt(ent string) string {
	switch ent {
	case "premium_interactions":
		return "premium"
	case "chat":
		return "chat"
	case "completions":
		return "completions"
	}
	return ent
}

// Copilot runs `gh api copilot_internal/user` (15 s) — gh's keyring holds the token.
func Copilot() Reading { return CopilotWith(proc.Find("gh")) }

// CopilotWith runs the given gh binary (fake-gh in tests).
func CopilotWith(exe string) Reading {
	if exe == "" {
		return Fail("Install gh and run `gh auth login` first.")
	}
	text, failed := ghOutput(exe, 15*time.Second)
	if failed != nil {
		return *failed
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "not logged in") || strings.Contains(lower, "not authenticated") {
		return Fail("Run `gh auth login` first.")
	}
	if strings.Contains(lower, "not found") && strings.Contains(lower, "404") {
		return Fail("This gh account has no Copilot.")
	}
	return ParseCopilot(text, time.Now().UTC())
}

func ghOutput(exe string, limit time.Duration) (string, *Reading) {
	g, err := proc.Spawn(proc.Hidden(exe, "api", "copilot_internal/user"))
	if err != nil {
		r := Fail(fmt.Sprintf("gh failed: %v", err))
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
	timedOut := func() (string, *Reading) {
		g.Kill()
		r := Fail("gh didn't answer in time.")
		return "", &r
	}
	if _, ok := g.WaitTimeout(limit); !ok {
		return timedOut()
	}
	var out [2]string
	for range 2 {
		select {
		case x := <-ch:
			out[x.i] = x.s
		case <-time.After(max(limit-time.Since(start), 0)):
			return timedOut()
		}
	}
	return out[0] + "\n" + out[1], nil
}
