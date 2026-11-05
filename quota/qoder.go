package quota

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-quota/internal/core"
)

// Qoder is the AI code editor at openapi.qoder.sh.
// The credential is kept in %APPDATA%\com.qoder.app.stable\auth.v1.dat, sealed with
// Chromium v10 AES-GCM keyed by Local State's DPAPI key.
// The sign-in is read in memory only and never refreshed.

const QoderURL = "https://openapi.qoder.sh/sash/api/v2/me/usage"

func QoderHome() string {
	base := core.AppData()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "com.qoder.app.stable")
}

func QoderAuth() string {
	if home := QoderHome(); home != "" {
		return filepath.Join(home, "auth.v1.dat")
	}
	return ""
}

func QoderLocalState() string {
	if home := QoderHome(); home != "" {
		return filepath.Join(home, "Local State")
	}
	return ""
}

func QoderPresent() bool {
	return presentFile(QoderAuth())
}

func Qoder(now time.Time) Reading {
	return QoderAt(QoderAuth(), QoderLocalState(), QoderURL, now)
}

func QoderAt(authPath, localStatePath, url string, now time.Time) Reading {
	if st, err := os.Stat(authPath); err != nil || !st.Mode().IsRegular() {
		return Fail("Qoder isn’t installed, or hasn’t been signed in to.")
	}
	token, expiresAt, err := qoderCredentials(authPath, localStatePath)
	if err != nil {
		return Fail(fmt.Sprintf("Couldn’t read Qoder’s sign-in: %v", err))
	}
	return QoderToken(url, token, expiresAt, now)
}

func QoderToken(url, token string, expiresAt *time.Time, now time.Time) Reading {
	if token == "" {
		return Fail("Sign in to Qoder first.")
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return Fail("Qoder's sign-in has expired — open Qoder to renew it.")
	}
	return QoderWith(url, token, now)
}

func qoderCredentials(authPath, localStatePath string) (string, *time.Time, error) {
	key, err := cursorOSCryptKey(localStatePath)
	if err != nil {
		return "", nil, err
	}
	sealed, err := os.ReadFile(authPath)
	if err != nil {
		return "", nil, err
	}
	plain, err := openCursorV10(sealed, key)
	if err != nil {
		return "", nil, err
	}
	return parseQoderAuthJSON(plain)
}

func parseQoderAuthJSON(text string) (string, *time.Time, error) {
	root, ok := parse(text)
	if !ok {
		return "", nil, fmt.Errorf("invalid auth JSON")
	}

	// token can be top-level or inside auth
	var token string
	if t := strOf(root, "token"); t != nil {
		token = *t
	} else if authObj, ok := objOf(root, "auth"); ok {
		if t := strOf(authObj, "token"); t != nil {
			token = *t
		}
	}

	var expires *time.Time
	// expiresAt can be top-level or in auth, and can be string (ISO) or number (ms/s)
	extractExpires := func(o core.JSON) *time.Time {
		if s := strOf(o, "expiresAt"); s != nil {
			if t, ok := ParseTime(*s); ok {
				return &t
			}
		}
		if n := numOf(o, "expiresAt"); n != nil {
			if *n > 1e11 {
				if t, ok := fromUnixMs(*n); ok {
					return &t
				}
			} else {
				if t, ok := fromUnixSecs(*n); ok {
					return &t
				}
			}
		}
		return nil
	}

	expires = extractExpires(root)
	if expires == nil {
		if authObj, ok := objOf(root, "auth"); ok {
			expires = extractExpires(authObj)
		}
	}

	return token, expires, nil
}

func QoderWith(url, token string, now time.Time) Reading {
	if r, blocked := checkBackoff(url, now); blocked {
		return r
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Fail("Couldn’t reach openapi.qoder.sh.")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Cosy-ClientType", "10")
	req.Header.Set("User-Agent", "Qoder")

	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return Fail("Couldn’t reach openapi.qoder.sh.")
	}
	defer res.Body.Close()

	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return Fail("Qoder’s answer couldn’t be read.")
	}

	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return Fail("openapi.qoder.sh refused Qoder’s sign-in — open Qoder to renew it.")
	case res.StatusCode == 429:
		return recordBackoff(url, "Qoder", res.Header.Get("Retry-After"), now)
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return Fail(fmt.Sprintf("openapi.qoder.sh answered %d.", res.StatusCode))
	}

	clearBackoff(url)
	return ParseQoderUsage(core.Lossy(b), now)
}

// ParseQoderUsage parses GET /sash/api/v2/me/usage response into Reading.
func ParseQoderUsage(text string, now time.Time) Reading {
	root, ok := parse(text)
	if !ok {
		return Fail("Qoder’s answer couldn’t be read.")
	}

	if dm := strOf(root, "displayMode"); dm != nil && strings.EqualFold(*dm, "enterprise") {
		return Fail("Qoder enterprise usage isn't shown here yet.")
	}

	qoderUsage, ok := objOf(root, "qoderUsage")
	if !ok {
		return Fail("Qoder didn’t report usage.")
	}

	isExceeded := false
	if excVal, ok := qoderUsage.Get("isQuotaExceeded"); ok {
		if b, err := excVal.Bool(); err == nil && b {
			isExceeded = true
		}
	}

	// Ring Used: Qoder's own UI headlines userQuota ("Plan credits").
	// If isQuotaExceeded is true, Used = 100.
	// Otherwise userQuota percentage * 100.
	// If userQuota is missing, fall back to totalUsagePercentage * 100.
	var headlineUsed *float64
	var details []Detail

	userQuota, hasUserQuota := objOf(qoderUsage, "userQuota")
	if hasUserQuota {
		usedVal := numOf(userQuota, "used")
		totalVal := numOf(userQuota, "total")
		pctVal := numOf(userQuota, "percentage")

		var pct float64
		if pctVal != nil {
			pct = *pctVal * 100
		} else if usedVal != nil && totalVal != nil && *totalVal > 0 {
			pct = *usedVal / *totalVal * 100
		}
		pct = min(max(pct, 0), 100)
		headlineUsed = &pct

		valStr := ""
		if usedVal != nil && totalVal != nil {
			valStr = fmt.Sprintf("%s%% used · %s / %s credits", Custom(pct, 0), Custom(*usedVal, 0), Custom(*totalVal, 0))
		} else {
			valStr = Custom(pct, 0) + "% used"
		}
		details = append(details, Detail{
			Label: "Plan credits",
			Value: valStr,
		})
	}

	if addOnQuota, hasAddOn := objOf(qoderUsage, "addOnQuota"); hasAddOn {
		usedVal := numOf(addOnQuota, "used")
		totalVal := numOf(addOnQuota, "total")
		pctVal := numOf(addOnQuota, "percentage")

		var pct float64
		if pctVal != nil {
			pct = *pctVal * 100
		} else if usedVal != nil && totalVal != nil && *totalVal > 0 {
			pct = *usedVal / *totalVal * 100
		}
		pct = min(max(pct, 0), 100)

		valStr := ""
		if usedVal != nil && totalVal != nil {
			valStr = fmt.Sprintf("%s%% used · %s / %s credits", Custom(pct, 0), Custom(*usedVal, 0), Custom(*totalVal, 0))
		} else {
			valStr = Custom(pct, 0) + "% used"
		}
		details = append(details, Detail{
			Label: "Add-on credits",
			Value: valStr,
		})
	}

	if headlineUsed == nil {
		if totVal := numOf(qoderUsage, "totalUsagePercentage"); totVal != nil {
			pct := min(max(*totVal*100, 0), 100)
			headlineUsed = &pct
		}
	}

	if headlineUsed == nil {
		return Fail("Qoder didn’t report plan usage.")
	}

	used := *headlineUsed
	if isExceeded {
		used = 100
	}

	// qoderUsage.expiresAt is the plan/trial expiration date
	var expiresAt *time.Time
	endsLabel := ""
	if expVal := numOf(qoderUsage, "expiresAt"); expVal != nil && *expVal > 0 {
		if *expVal > 1e11 {
			if t, ok := fromUnixMs(*expVal); ok {
				expiresAt = &t
			}
		} else {
			if t, ok := fromUnixSecs(*expVal); ok {
				expiresAt = &t
			}
		}
	} else if s := strOf(qoderUsage, "expiresAt"); s != nil {
		if t, ok := ParseTime(*s); ok {
			expiresAt = &t
		}
	}

	if expiresAt != nil {
		endsLabel = FormatLocal(*expiresAt, "d MMM")
		details = append(details, Detail{
			Label: "Plan ends",
			Value: endsLabel,
		})
	}

	plan := ""
	if ut := strOf(qoderUsage, "userType"); ut != nil && *ut != "" {
		plan = parseQoderPlan(*ut)
	}

	detail := ""
	if plan != "" {
		detail = plan + " · "
	}
	detail += Custom(used, 0) + "% of plan"
	if endsLabel != "" {
		detail += " · ends " + endsLabel
	}

	r := newReading(used, detail)
	r.Plan = plan
	r.Summary = "Plan credits"
	r.Details = details
	return r
}

func parseQoderPlan(userType string) string {
	s := strings.TrimPrefix(userType, "personal_")
	s = strings.TrimPrefix(s, "personal-")

	hasTrial := false
	if strings.HasSuffix(s, "_trial") {
		hasTrial = true
		s = strings.TrimSuffix(s, "_trial")
	} else if strings.HasSuffix(s, "-trial") {
		hasTrial = true
		s = strings.TrimSuffix(s, "-trial")
	}

	switch strings.ToLower(s) {
	case "professional":
		s = "Pro"
	default:
		parts := strings.Split(s, "_")
		for i, p := range parts {
			parts[i] = capital(p)
		}
		s = strings.Join(parts, " ")
	}

	if hasTrial {
		s += " Trial"
	}
	return s
}
