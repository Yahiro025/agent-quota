package quota

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
	for _, key := range []string{"copilot_plan", "plan"} {
		if p := strOf(root, key); p != nil && *p != "" {
			plan = capital(*p)
			break
		}
	}
	resetText := ""
	var resetAt time.Time
	resetSource := ""
	if r := strOf(root, "quota_reset_date"); r != nil && *r != "" {
		resetSource = *r
		if len(*r) > 10 {
			if t, ok := ParseTime(*r); ok {
				resetAt = t
			}
		}
		if t, ok := ParseTime(*r); ok {
			resetText = " · resets " + FormatLocal(t, "d MMM")
		} else if len(*r) >= 10 {
			if t, err := time.Parse("2006-01-02", (*r)[:10]); err == nil {
				resetText = " · resets " + FormatLocal(t, "d MMM")
			}
		}
	}
	if r := strOf(root, "quota_reset_date_utc"); r != nil {
		if t, err := time.Parse(time.RFC3339Nano, *r); err == nil {
			resetAt = t.UTC()
			resetText = " · resets " + FormatLocal(resetAt, "d MMM")
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
	var details []Detail
	var observedAt time.Time
	unlimited := 0
	for _, p := range props {
		s := p.Val
		row := Detail{Label: shortEnt(p.Key), ResetAt: resetAt}
		if epoch := numOf(s, "quota_reset_at"); epoch != nil && *epoch > 0 {
			if t, ok := fromUnixSecs(*epoch); ok && t.After(now) {
				row.ResetAt = t
			}
		}
		if row.ResetAt.IsZero() {
			row.ResetText = resetSource
		}
		if stamp := strOf(s, "timestamp_utc"); stamp != nil {
			if t, err := time.Parse(time.RFC3339Nano, *stamp); err == nil && t.After(observedAt) {
				observedAt = t.UTC()
			}
		}
		isUnlimited := false
		if u, ok := s.Get("unlimited"); ok {
			if b, err := u.Bool(); err == nil && b {
				isUnlimited = true
				unlimited++
			}
		}
		rem := numOf(s, "percent_remaining")
		remaining := numOf(s, "quota_remaining")
		if remaining == nil {
			remaining = numOf(s, "remaining")
		}
		entitlement, limit, credits := numOf(s, "entitlement"), numOf(s, "limit"), numOf(s, "credits_used")
		if limit != nil && *limit <= 0 {
			limit = nil
		}
		if entitlement != nil && *entitlement <= 0 {
			entitlement = nil
		}
		var facts []string
		if isUnlimited {
			row.Value = "Unlimited"
		} else {
			if rem != nil {
				used := min(max(100-*rem, 0), 100)
				facts = append(facts, Custom(used, 0)+"% used")
				if used > top {
					top = used
					detailEnt = fmt.Sprintf("%s %s%%", shortEnt(p.Key), Custom(used, 0))
					if entitlement == nil && credits == nil && remaining != nil && limit != nil {
						detailEnt = fmt.Sprintf("%s %s/%s", shortEnt(p.Key), Custom(*limit-*remaining, 0), Custom(*limit, 0))
					}
				}
			}
			if credits != nil {
				facts = append(facts, strconv.FormatFloat(*credits, 'f', -1, 64)+" used")
			}
			if entitlement != nil {
				facts = append(facts, strconv.FormatFloat(*entitlement, 'f', -1, 64)+" entitlement")
			} else if limit != nil {
				if rem != nil && remaining != nil && credits == nil {
					facts = append(facts, Custom(*limit-*remaining, 2)+" of "+Custom(*limit, 2)+" used")
				} else if rem != nil {
					facts = append(facts, Custom(*limit, 2)+" limit")
				}
			}
			if remaining != nil {
				facts = append(facts, strconv.FormatFloat(*remaining, 'f', -1, 64)+" remaining")
			}
			if rem == nil && entitlement == nil && limit != nil {
				facts = append(facts, Custom(*limit, 2)+" limit")
			}
			row.Value = strings.Join(facts, " · ")
		}
		if row.Value != "" {
			details = append(details, row)
		}
		var overage []string
		if permitted, ok := s.Get("overage_permitted"); ok {
			if on, err := permitted.Bool(); err == nil && (on || !isUnlimited) {
				if on {
					overage = append(overage, "On")
				} else {
					overage = append(overage, "Off")
				}
			}
		}
		for _, metric := range []struct{ key, suffix string }{{"overage_count", " used"}, {"overage_entitlement", " entitlement"}} {
			if value := numOf(s, metric.key); value != nil && *value > 0 {
				overage = append(overage, strconv.FormatFloat(*value, 'f', -1, 64)+metric.suffix)
			}
		}
		if len(overage) != 0 {
			details = append(details, Detail{Label: row.Label + " overage", Value: strings.Join(overage, " · "), ResetAt: row.ResetAt, ResetText: row.ResetText})
		}
	}
	if top < 0 {
		if unlimited == 0 || unlimited != len(props) {
			return Fail("Copilot didn't report plan usage.")
		}
		r := newReading(0, "Unlimited")
		if plan != "" {
			r.Detail = plan + " · Unlimited"
		}
		r.Plan, r.Summary, r.Details = plan, "Unlimited", details
		r.ObservedAt = observedAt
		r.ResetLabel = strings.TrimPrefix(resetText, " · resets ")
		return r
	}
	detail := ""
	if plan != "" {
		detail = plan + " · "
	}
	r := newReading(top, detail+detailEnt+resetText)
	r.Plan, r.Summary = plan, detailEnt
	r.Details = details
	r.ObservedAt = observedAt
	r.ResetLabel = strings.TrimPrefix(resetText, " · resets ")
	return r
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
