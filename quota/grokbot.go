package quota

import (
	b64enc "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-quota/internal/core"
)

// Grok Bot is Cursor's desktop Grok client. Its weekly usage is its own quota,
// separate from the Cursor plan percentage: GetSandUsageStatus on Cursor's API
// returns usagePercent plus nextResetTimestampUtc. The sign-in is the active
// account Cursor's safeStorage sealed into sand-secrets.json; like Cursor's, it
// is read in memory only and never refreshed.

// GrokBotHome is where Grok Bot keeps its Electron state, beside the Cursor
// database it shares the safeStorage format with.
func GrokBotHome() string {
	base := core.AppData()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Grok Bot")
}

func GrokBotSecrets() string {
	if home := GrokBotHome(); home != "" {
		return filepath.Join(home, "sand-secrets.json")
	}
	return ""
}

func GrokBotPresent() bool { return presentFile(GrokBotSecrets()) }

const GrokBotURL = "https://api2.cursor.sh/aiserver.v1.DashboardService/GetSandUsageStatus"

func GrokBot() Reading {
	return GrokBotAt(GrokBotSecrets(), GrokBotHome(), GrokBotURL, time.Now().UTC())
}

func GrokBotAt(secrets, home, url string, now time.Time) Reading {
	if st, err := os.Stat(secrets); err != nil || !st.Mode().IsRegular() {
		return Fail("Grok Bot isn’t installed, or hasn’t been signed in to.")
	}
	token, err := grokBotToken(secrets, filepath.Join(home, "Local State"))
	if err != nil {
		return Fail(fmt.Sprintf("Couldn’t read Grok Bot’s sign-in: %v", err))
	}
	if token == nil {
		return Fail("Sign in to Grok Bot first.")
	}
	return GrokBotWith(url, *token, now)
}

func GrokBotWith(url, token string, now time.Time) Reading {
	return grokBotUsage(url, token, now)
}

// grokBotToken is the active account's cursor-access-token: base64 of the same
// Chromium v10 blob Cursor seals its own token in.
func grokBotToken(secrets, localState string) (*string, error) {
	b, err := core.ReadFile(secrets)
	if err != nil {
		return nil, err
	}
	var top struct {
		Accounts string `json:"cursor-accounts"`
	}
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	var accounts struct {
		Active   string `json:"active"`
		Accounts map[string]struct {
			Token string `json:"cursor-access-token"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(top.Accounts), &accounts); err != nil {
		return nil, err
	}
	sealedText := accounts.Accounts[accounts.Active].Token
	if sealedText == "" {
		return nil, nil
	}
	sealed, err := b64enc.StdEncoding.DecodeString(sealedText)
	if err != nil {
		return nil, err
	}
	key, err := cursorOSCryptKey(localState)
	if err != nil {
		return nil, err
	}
	token, err := openCursorV10(sealed, key)
	if err != nil {
		return nil, err
	}
	if t, ok := CleanToken(token); ok {
		return &t, nil
	}
	return nil, nil
}

func grokBotUsage(url, token string, now time.Time) Reading {
	if r, blocked := checkBackoff(url, now); blocked {
		return r
	}
	req, err := http.NewRequest("POST", url, strings.NewReader("{}"))
	if err != nil {
		return Fail("Couldn’t reach api2.cursor.sh.")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Hover")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Fail("Couldn’t reach api2.cursor.sh.")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return Fail("Grok Bot’s answer couldn’t be read.")
	}
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return Fail("api2.cursor.sh refused Grok Bot’s sign-in — open Grok Bot to renew it.")
	case res.StatusCode == 429:
		return recordBackoff(url, "api2.cursor.sh", res.Header.Get("Retry-After"), now)
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return Fail(fmt.Sprintf("api2.cursor.sh answered %d.", res.StatusCode))
	}
	clearBackoff(url)
	return ParseGrokBotUsage(core.Lossy(b))
}

// ParseGrokBotUsage reads GetSandUsageStatus: one weekly percentage, its reset,
// and the plan labels the desktop app shows.
func ParseGrokBotUsage(text string) Reading {
	root, ok := parse(text)
	if !ok {
		return Fail("Grok Bot’s answer couldn’t be read.")
	}
	percent := numOf(root, "usagePercent")
	if percent == nil {
		return Fail("Grok Bot didn’t report weekly usage.")
	}
	used := min(max(*percent, 0), 100)
	plan := ""
	if p := strOf(root, "cursorPlanName"); p != nil && *p != "" {
		plan = capital(*p)
	} else if p := strOf(root, "grokPlanLabel"); p != nil && *p != "" {
		plan = capital(*p)
	}
	var reset *time.Time
	resetLabel := ""
	if e := strOf(root, "nextResetTimestampUtc"); e != nil {
		if t, ok := ParseTime(*e); ok {
			reset, resetLabel = &t, FormatLocal(t, "d MMM")
		}
	}
	detail := ""
	if plan != "" {
		detail = plan + " · "
	}
	detail += Custom(used, 0) + "% of week"
	if resetLabel != "" {
		detail += " · resets " + resetLabel
	}
	r := newReading(used, detail)
	r.Plan, r.Summary, r.ResetLabel = plan, "Weekly usage", resetLabel
	r.Details = []Detail{percentDetail("week", used, reset)}
	return r
}
