package quota

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPureParseAntigravityUsesEnabledGeminiAndClaudeGPTQuotas(t *testing.T) {
	input := `{"status":"SUCCESS","response":"ignored display text","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":0.72,"reset_time":"2026-10-20T08:00:00Z"},{"id":"gemini-5h","window":"5h","remaining_fraction":0.1,"disabled":true}]},{"name":"Claude and GPT models","buckets":[{"id":"3p-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":0.2,"reset_time":"2026-10-19T08:00:00Z"}]}]}}}`
	r := ParseAntigravity(input, time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC))
	if !r.OK() || *r.Used != 80 {
		t.Fatalf("used = %v, want 80%% from most-used enabled quota bucket", r.Used)
	}
	for _, want := range []string{"Gemini Models", "Claude and GPT models", "80%", "28%", "resets"} {
		if !strings.Contains(r.Detail, want) {
			t.Errorf("detail %q does not contain %q", r.Detail, want)
		}
	}
	if strings.Contains(r.Detail, "5h") {
		t.Errorf("disabled bucket appeared in detail: %q", r.Detail)
	}
}

func TestParseAntigravityRejectsQuotaAnsweredAsModelTurn(t *testing.T) {
	input := `{"status":"SUCCESS","num_turns":1,"conversation_id":"spent-a-turn","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.9}]}]}}}`
	if r := ParseAntigravity(input, time.Now()); r.OK() {
		t.Fatalf("accepted a quota response that also reports a model turn: %#v", r)
	}
}

func TestParseAntigravityRequiresSuccessfulUsageCommand(t *testing.T) {
	for _, input := range []string{
		`{"status":"ERROR","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"x","remaining_fraction":0.4}]}]}}}`,
		`{"status":"SUCCESS","command":{"name":"quota","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"x","remaining_fraction":0.4}]}]}}}`,
		`{"status":"SUCCESS","response":"Gemini Models 40%","num_turns":0,"conversation_id":""}`,
	} {
		if r := ParseAntigravity(input, time.Now()); r.OK() {
			t.Errorf("accepted a non-usage or display-only response: %#v", r)
		}
	}
}

func TestPureParseAntigravityRejectsMissingOrInvalidQuota(t *testing.T) {
	for _, input := range []string{
		`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[]}}}`,
		`{"status":"SUCCESS","command":{"name":"other","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"x","remaining_fraction":0.4}]}]}}}`,
		`{"status":"SUCCESS","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"x","remaining_fraction":1.1}]}]}}}`,
	} {
		if r := ParseAntigravity(input, time.Now()); r.OK() {
			t.Errorf("accepted invalid quota envelope: %s", input)
		}
	}
}

func TestAntigravityStructuredUnauthenticatedErrorGuidesWithoutLeakingOutput(t *testing.T) {
	withAntigravityStub(t, `@echo off
if "%1"=="--version" (
 echo 1.3.4
 exit /b 0
)
>>"%AGY_MARKER%" echo usage
echo AGY_ERROR: {"error_code":401,"message":"expired-key-value"}
exit /b 1
`)
	t.Setenv("AGY_VERSION", "1.3.4")
	r := Antigravity()
	if r.OK() || !strings.Contains(strings.ToLower(r.Detail), "open agy to renew") {
		t.Fatalf("structured authentication failure wasn't actionable: %#v", r)
	}
	if strings.Contains(strings.ToLower(r.Detail), "agy login") || strings.Contains(r.Detail, "expired-key-value") {
		t.Fatalf("failure guidance exposed an unsupported command or raw output: %q", r.Detail)
	}
}

func TestAntigravityStopsAtOutputLimit(t *testing.T) {
	withAntigravityStub(t, `@echo off
if "%1"=="--version" (
 echo 1.3.4
 exit /b 0
)
for /L %%i in (1,1,20000) do echo 123456789012345678901234567890
`)
	t.Setenv("AGY_VERSION", "1.3.4")
	if r := Antigravity(); r.OK() || !strings.Contains(r.Detail, "too much output") {
		t.Fatalf("oversized CLI response wasn't rejected at the capture limit: %#v", r)
	}
}

func TestRunAntigravityKillsTimedOutCLI(t *testing.T) {
	withAntigravityStub(t, `@echo off
:again
goto again
`)
	r := runAntigravity("agy.cmd", nil, 20*time.Millisecond)
	if !r.timedOut {
		t.Fatalf("hung CLI did not time out: %#v", r)
	}
}

func TestAntigravityStubCLIEnforcesVersionDispatchExitAndModelTurnLatch(t *testing.T) {
	marker := withAntigravityStub(t, `@echo off
if "%1"=="--version" (
 echo %AGY_VERSION%
 exit /b 0
)
if not "%1"=="-p" exit /b 8
if not "%2"=="/usage" exit /b 8
if not "%3"=="--output-format" exit /b 8
if not "%4"=="json" exit /b 8
if not "%5"=="--print-timeout" exit /b 8
if not "%6"=="20s" exit /b 8
if not "%7"=="" exit /b 8
>>"%AGY_MARKER%" echo usage
if "%AGY_MODE%"=="quota" (
 echo {"status":"SUCCESS","num_turns":0,"conversation_id":"","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.75,"reset_time":"2026-10-20T08:00:00Z"}]}]}}}
 exit /b 0
)
if "%AGY_MODE%"=="badexit" (
 echo quota request failed
 exit /b 7
)
if "%AGY_MODE%"=="turn" (
 echo {"status":"SUCCESS","num_turns":1,"conversation_id":"prompt-turn","command":{"name":"usage","data":{"groups":[{"name":"Gemini Models","buckets":[{"id":"gemini-weekly","window":"weekly","remaining_fraction":0.75}]}]}}}
 exit /b 0
)
exit /b 9
`)
	t.Setenv("AGY_VERSION", "1.1.10")
	t.Setenv("AGY_MODE", "quota")
	if r := Antigravity(); r.OK() || !strings.Contains(r.Detail, "1.1.11") {
		t.Fatalf("old CLI version was not rejected: %#v", r)
	}
	if data, err := os.ReadFile(marker); err == nil && len(data) != 0 {
		t.Fatalf("old version still ran /usage: %q", data)
	}

	t.Setenv("AGY_VERSION", "1.3.4")
	if r := Antigravity(); !r.OK() || *r.Used != 25 || !strings.Contains(r.Detail, "Gemini Models weekly 25%") {
		t.Fatalf("supported CLI didn't produce the quota reading: %#v", r)
	}

	t.Setenv("AGY_MODE", "badexit")
	if r := Antigravity(); r.OK() || !strings.Contains(r.Detail, "exit 7") {
		t.Fatalf("nonzero CLI exit was accepted or hidden: %#v", r)
	}

	t.Setenv("AGY_MODE", "turn")
	if r := Antigravity(); r.OK() || !strings.Contains(r.Detail, "model turn") {
		t.Fatalf("model turn was accepted or not reported: %#v", r)
	}
	data, err := os.ReadFile(marker)
	if err != nil || strings.Count(string(data), "usage") != 3 {
		t.Fatalf("expected three usage dispatches before the latch, got %q (err %v)", data, err)
	}
	if r := Antigravity(); r.OK() {
		t.Fatalf("latched reader unexpectedly succeeded: %#v", r)
	}
	after, err := os.ReadFile(marker)
	if err != nil || strings.Count(string(after), "usage") != 3 {
		t.Fatalf("latched reader ran agy a second time: %q (err %v)", after, err)
	}
}

func withAntigravityStub(t *testing.T, body string) string {
	t.Helper()
	antigravityUnsupported.Lock()
	wasLatched := antigravityUnsupported.latched
	antigravityUnsupported.latched = false
	antigravityUnsupported.Unlock()
	t.Cleanup(func() {
		antigravityUnsupported.Lock()
		antigravityUnsupported.latched = wasLatched
		antigravityUnsupported.Unlock()
	})
	dir := t.TempDir()
	path := filepath.Join(dir, "agy.cmd")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "calls.txt")
	t.Setenv("AGY_MARKER", marker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("SystemRoot")+`\System32`)
	t.Setenv("PATHEXT", ".CMD;.EXE;.BAT")
	return marker
}
