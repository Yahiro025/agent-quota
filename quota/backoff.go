package quota

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// backoffEntry records an active 429 rate-limit window for a remote host.
// The service name and expiration instant are stored so subsequent reads within
// the window rebuild the failure message dynamically with the remaining time.
type backoffEntry struct {
	until   time.Time
	service string
}

var (
	backoffMu      sync.Mutex
	backoffEntries = map[string]backoffEntry{}
)

// ResetBackoff clears all rate-limit backoff records. Used between tests.
func ResetBackoff() {
	backoffMu.Lock()
	defer backoffMu.Unlock()
	backoffEntries = map[string]backoffEntry{}
}

// backoffKey normalizes a URL or host into a lowercase host key for the backoff map.
func backoffKey(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	if u, err := url.Parse("http://" + rawURL); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	return strings.ToLower(strings.TrimSpace(rawURL))
}

// checkBackoff checks whether target is currently rate-limited at now. If blocked,
// it rebuilds the failure reading with the updated remaining wait time; otherwise Reading{} and false.
func checkBackoff(target string, now time.Time) (Reading, bool) {
	key := backoffKey(target)
	backoffMu.Lock()
	defer backoffMu.Unlock()
	entry, ok := backoffEntries[key]
	if !ok {
		return Reading{}, false
	}
	if now.Before(entry.until) {
		remaining := entry.until.Sub(now)
		msg := fmt.Sprintf("%s is limiting usage checks; trying again in %s.", entry.service, retryMinutes(remaining))
		return Fail(msg), true
	}
	delete(backoffEntries, key)
	return Reading{}, false
}

// clearBackoff removes any rate-limit block for target (e.g. after a successful response).
func clearBackoff(target string) {
	key := backoffKey(target)
	backoffMu.Lock()
	defer backoffMu.Unlock()
	delete(backoffEntries, key)
}

// parseRetryAfter parses a Retry-After header, which can be delta-seconds or an HTTP-date.
// It returns the duration relative to now, or (0, false) if missing or malformed.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(header); err == nil && sec >= 0 {
		if sec > 3600 {
			sec = 3600
		}
		return time.Duration(sec) * time.Second, true
	} else if err != nil {
		if numErr, ok := err.(*strconv.NumError); ok && numErr.Err == strconv.ErrRange && !strings.HasPrefix(header, "-") {
			return 3600 * time.Second, true
		}
	}
	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// retryMinutes formats backoff duration rounded up to minutes.
// Ceiling of remaining minutes: 1 minute or less returns "a minute",
// 5 minutes returns "five minutes" to keep existing messaging consistent,
// and all other values return "N minutes".
func retryMinutes(d time.Duration) string {
	mins := int(math.Ceil(d.Minutes()))
	if mins <= 1 {
		return "a minute"
	}
	if mins == 5 {
		return "five minutes"
	}
	return fmt.Sprintf("%d minutes", mins)
}

// recordBackoff registers a 429 block for target at now, honoring Retry-After when
// present. Backoff is max(now+5min, now+Retry-After) capped at 1 hour. It returns
// the corresponding Failure reading.
func recordBackoff(target, service, retryAfter string, now time.Time) Reading {
	key := backoffKey(target)
	delay := 5 * time.Minute
	if retry, ok := parseRetryAfter(retryAfter, now); ok && retry > delay {
		delay = retry
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	until := now.Add(delay)
	msg := fmt.Sprintf("%s is limiting usage checks; trying again in %s.", service, retryMinutes(delay))
	r := Fail(msg)

	backoffMu.Lock()
	defer backoffMu.Unlock()
	backoffEntries[key] = backoffEntry{
		until:   until,
		service: service,
	}
	return r
}
