package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-quota/internal/core"
	"agent-quota/internal/proc"
)

// Command Code is the CLI assistant with billing and window limits at api.commandcode.ai.
// The apiKey is read from %USERPROFILE%\.commandcode\auth.json read-only, never logged,
// and never cached.

const CommandCodeBaseURL = "https://api.commandcode.ai"

func CommandCodeHome() string {
	return filepath.Join(proc.Home(), ".commandcode")
}

func CommandCodeAuth() string {
	return filepath.Join(CommandCodeHome(), "auth.json")
}

func CommandCodePresent() bool {
	return presentFile(CommandCodeAuth())
}

func CommandCode(now time.Time) Reading {
	return CommandCodeAt(CommandCodeAuth(), CommandCodeBaseURL, now)
}

func CommandCodeAt(authPath, baseURL string, now time.Time) Reading {
	if st, err := os.Stat(authPath); err != nil || !st.Mode().IsRegular() {
		return Fail("Sign in to Command Code first.")
	}
	b, err := core.ReadFile(authPath)
	if err != nil {
		return Fail(fmt.Sprintf("Couldn’t read Command Code’s sign-in: %v", err))
	}
	var auth struct {
		ApiKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(b, &auth); err != nil {
		return Fail("Couldn’t read Command Code’s sign-in.")
	}
	if strings.TrimSpace(auth.ApiKey) == "" {
		return Fail("Sign in to Command Code first.")
	}
	return CommandCodeWith(auth.ApiKey, baseURL, now)
}

func CommandCodeWith(apiKey, baseURL string, now time.Time) Reading {
	creditsURL := baseURL + "/alpha/billing/credits"
	if r, blocked := checkBackoff(creditsURL, now); blocked {
		return r
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	creditsBody, status, header, ok := commandCodeGet(ctx, creditsURL, apiKey)
	switch {
	case !ok:
		return Fail("Couldn’t reach api.commandcode.ai.")
	case status == 401 || status == 403:
		return Fail("Command Code refused your sign-in — run commandcode to renew it.")
	case status == 429:
		return recordBackoff(creditsURL, "Command Code", header.Get("Retry-After"), now)
	case status < 200 || status >= 300:
		return Fail(fmt.Sprintf("api.commandcode.ai answered %d.", status))
	}
	clearBackoff(creditsURL)

	// Subscriptions endpoint gives the planId and currentPeriodEnd.
	// A failure here does not discard the usage reading.
	var planId, currentPeriodEnd string
	subsURL := baseURL + "/alpha/billing/subscriptions"
	if subsBody, sStatus, sHeader, sOK := commandCodeGet(ctx, subsURL, apiKey); sOK {
		if sStatus == 429 {
			recordBackoff(subsURL, "Command Code", sHeader.Get("Retry-After"), now)
		} else if sStatus == 200 {
			var subResp struct {
				Success bool `json:"success"`
				Data    struct {
					PlanId           string `json:"planId"`
					CurrentPeriodEnd string `json:"currentPeriodEnd"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(subsBody), &subResp); err == nil && subResp.Success {
				planId = subResp.Data.PlanId
				currentPeriodEnd = subResp.Data.CurrentPeriodEnd
			}
		}
	}

	return ParseCommandCode(creditsBody, planId, currentPeriodEnd, now)
}

func commandCodeGet(ctx context.Context, url, apiKey string) (string, int, http.Header, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", 0, nil, false
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agent-quota")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, nil, false
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return "", 0, nil, false
	}
	return core.Lossy(b), res.StatusCode, res.Header, true
}

// ParseCommandCode parses the credits response into a Reading.
func ParseCommandCode(creditsJSON, planId, currentPeriodEnd string, now time.Time) Reading {
	root, ok := parse(creditsJSON)
	if !ok {
		return Fail("Command Code’s answer couldn’t be read.")
	}

	windowLimits, ok := objOf(root, "windowLimits")
	if !ok {
		return Fail("Command Code reported no usage limits.")
	}

	if limitedVal, ok := windowLimits.Get("limited"); ok {
		if b, err := limitedVal.Bool(); err == nil && !b {
			plan := parseCommandCodePlan(planId)
			detail := "Unlimited"
			if plan != "" {
				detail = plan + " · Unlimited"
			} else {
				plan = "Unlimited"
			}
			r := newReading(0, detail)
			r.Plan = plan
			r.Summary = "Unlimited"
			if creditsObj, hasCredits := objOf(root, "credits"); hasCredits {
				monthly := 0.0
				if m := numOf(creditsObj, "monthlyCredits"); m != nil {
					monthly = *m
				}
				purchased := 0.0
				if p := numOf(creditsObj, "purchasedCredits"); p != nil {
					purchased = *p
				}
				free := 0.0
				if f := numOf(creditsObj, "freeCredits"); f != nil {
					free = *f
				}
				remaining := monthly + purchased + free
				d := Detail{
					Label: "Monthly credits",
					Value: Custom(remaining, 2) + " remaining",
				}
				if currentPeriodEnd != "" {
					if t, ok := ParseTime(currentPeriodEnd); ok {
						d.ResetAt = t
					}
				}
				r.Details = append(r.Details, d)
			}
			return r
		}
	}

	type rawWindow struct {
		used  float64
		reset *time.Time
		ok    bool
	}

	readWindow := func(name string) rawWindow {
		o, ok := objOf(windowLimits, name)
		if !ok {
			return rawWindow{}
		}
		usedVal := numOf(o, "used")
		capVal := numOf(o, "cap")
		if usedVal == nil || capVal == nil || *capVal <= 0 {
			return rawWindow{}
		}
		pct := min(max(*usedVal / *capVal * 100, 0), 100)
		var reset *time.Time
		if r := numOf(o, "resetAt"); r != nil && *r > 0 {
			if t, ok := fromUnixMs(*r); ok {
				reset = &t
			}
		}
		if reset != nil && !reset.After(now) {
			pct = 0
			reset = nil
		}
		return rawWindow{used: pct, reset: reset, ok: true}
	}

	w5h := readWindow("fiveHour")
	wWeek := readWindow("weekly")

	if !w5h.ok && !wWeek.ok {
		return Fail("Command Code reported no usage limits.")
	}

	var windows []Window
	var details []Detail
	topUsed := 0.0
	var topReset *time.Time
	var reset5h time.Time

	if w5h.ok {
		windows = append(windows, Window{Label: "5h", Used: w5h.used})
		details = append(details, percentDetail("5h", w5h.used, w5h.reset))
		topUsed = w5h.used
		topReset = w5h.reset
		if w5h.reset != nil && w5h.reset.After(now) {
			reset5h = *w5h.reset
		}
	}

	if wWeek.ok {
		windows = append(windows, Window{Label: "week", Used: wWeek.used})
		details = append(details, percentDetail("week", wWeek.used, wWeek.reset))
		if !w5h.ok || wWeek.used > topUsed || (topReset == nil && wWeek.reset != nil) {
			topUsed = wWeek.used
			topReset = wWeek.reset
		}
	}

	// Monthly credits detail: remaining = monthlyCredits + purchasedCredits + freeCredits
	creditsObj, hasCredits := objOf(root, "credits")
	if hasCredits {
		monthly := 0.0
		if m := numOf(creditsObj, "monthlyCredits"); m != nil {
			monthly = *m
		}
		purchased := 0.0
		if p := numOf(creditsObj, "purchasedCredits"); p != nil {
			purchased = *p
		}
		free := 0.0
		if f := numOf(creditsObj, "freeCredits"); f != nil {
			free = *f
		}
		remaining := monthly + purchased + free
		d := Detail{
			Label: "Monthly credits",
			Value: Custom(remaining, 2) + " remaining",
		}
		if currentPeriodEnd != "" {
			if t, ok := ParseTime(currentPeriodEnd); ok {
				d.ResetAt = t
			}
		}
		details = append(details, d)
	}

	plan := parseCommandCodePlan(planId)
	planText := ""
	if plan != "" {
		planText = plan + " · "
	}

	parts := make([]string, len(windows))
	for i, w := range windows {
		parts[i] = fmt.Sprintf("%s %s%%", w.Label, Custom(w.Used, 0))
	}

	resetText := ""
	if topReset != nil {
		shown := FormatLocal(*topReset, "d MMM HH:mm")
		if sameLocalDay(*topReset, now) {
			shown = FormatLocal(*topReset, "HH:mm")
		}
		resetText = " · resets " + shown
	}

	detail := planText + strings.Join(parts, " · ") + resetText
	r := newReading(topUsed, detail)
	r.Reset5h = reset5h
	r.Plan = plan
	r.Summary = "Usage limits"
	r.Windows = windows
	r.Details = details
	return r
}

func parseCommandCodePlan(planId string) string {
	if planId == "" {
		return ""
	}
	isTeams := strings.HasPrefix(planId, "teams-")
	s := strings.TrimPrefix(planId, "individual-")
	s = strings.TrimPrefix(s, "teams-")

	// Strip trailing -vN (e.g. -v1)
	if idx := strings.LastIndex(s, "-v"); idx >= 0 {
		rest := s[idx+2:]
		allDigits := true
		for _, c := range rest {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(rest) > 0 {
			s = s[:idx]
		}
	}

	parts := strings.Split(s, "-")
	for i, p := range parts {
		parts[i] = capital(p)
	}
	name := strings.Join(parts, " ")
	if isTeams {
		name = "Teams " + name
	}
	return name
}
