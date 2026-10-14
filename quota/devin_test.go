package quota

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDevin(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	r := ParseDevin(`{"plan":"Pro","dailyQuotaRemainingPercent":38,"weeklyQuotaRemainingPercent":69}`, now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}
	if got := *r.Used; got != 62 {
		t.Fatalf("used=%v, want 62", got)
	}
	if !strings.Contains(r.Detail, "Pro") || !strings.Contains(r.Detail, "day") {
		t.Fatalf("detail=%q", r.Detail)
	}
}

func TestParseDevinNestedPlanStatus(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour).Unix()
	text := fmt.Sprintf(`{"userStatus":{"planStatus":{"planInfo":{"planName":"pro"},"dailyQuotaRemainingPercent":100,"weeklyQuotaRemainingPercent":66,"dailyQuotaResetAtUnix":"%d","weeklyQuotaResetAtUnix":"%d"}}}`, reset, reset)
	r := ParseDevin(text, now)
	if !r.OK() {
		t.Fatalf("not OK: %q", r.Detail)
	}
	if got := *r.Used; got != 34 {
		t.Fatalf("used=%v, want max(daily 0, weekly 34)=34", got)
	}
	for _, part := range []string{"Pro", "day 0%", "week 34%", "resets"} {
		if !strings.Contains(r.Detail, part) {
			t.Fatalf("detail %q does not contain %q", r.Detail, part)
		}
	}
}

func TestParseDevinMissingNestedQuotasFails(t *testing.T) {
	r := ParseDevin(`{"userStatus":{"planStatus":{"planInfo":{"planName":"pro"}}}}`, time.Now())
	if r.OK() {
		t.Fatalf("missing quotas reported as usage: %#v", r)
	}
}

func TestParseDevinCreds(t *testing.T) {
	c, ok := parseDevinCreds("windsurf_api_key = \"k123\"\napi_server_url = \"https://server.codeium.com\"\n")
	if !ok || c.key != "k123" || c.server != "https://server.codeium.com" {
		t.Fatalf("got %+v %v", c, ok)
	}
	if _, ok := parseDevinCreds("windsurf_api_key=\"\""); ok {
		t.Fatalf("expected failure on empty key")
	}
}

func TestDevinServer(t *testing.T) {
	for _, code := range []int{200, 401, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Connect-Protocol-Version") != "1" {
				t.Errorf("missing connect header")
			}
			w.WriteHeader(code)
			fmt.Fprint(w, `{"plan":"Pro","dailyQuotaRemainingPercent":38,"weeklyQuotaRemainingPercent":69}`)
		}))
		r := DevinWith(srv.URL, "k", "1.2.3", time.Now().UTC())
		srv.Close()
		switch code {
		case 200:
			if !r.OK() {
				t.Fatalf("200 not OK: %q", r.Detail)
			}
		default:
			if r.OK() {
				t.Fatalf("%d expected failure", code)
			}
			if code == 500 && strings.Contains(strings.ToLower(r.Detail), "version") {
				t.Fatalf("500 reason guessed a version problem: %q", r.Detail)
			}
		}
	}
}

func TestDevinWithReleasedMetadata(t *testing.T) {
	const version = "3000.11.3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exa.seat_management_pb.SeatManagementService/GetUserStatus" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if got := r.Header.Get("Connect-Protocol-Version"); got != "1" {
			t.Errorf("Connect-Protocol-Version=%q, want 1", got)
		}
		var request struct {
			Metadata map[string]any `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		want := map[string]string{
			"apiKey": "synthetic-key", "ideName": "devin-cli", "ideType": "chisel",
			"ideVersion": version, "extensionName": "chisel", "extensionVersion": version,
			"locale": "en", "os": "windows", "userJwt": "",
		}
		for name, expected := range want {
			if got, ok := request.Metadata[name].(string); !ok || got != expected {
				t.Errorf("metadata[%q]=%v, want %q", name, request.Metadata[name], expected)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"userStatus":{"planStatus":{"planInfo":{"planName":"pro"},"dailyQuotaRemainingPercent":100,"weeklyQuotaRemainingPercent":66,"dailyQuotaResetAtUnix":"1791896400","weeklyQuotaResetAtUnix":"1791896400"}}}`)
	}))
	defer srv.Close()

	r := DevinWith(srv.URL, "synthetic-key", version, time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC))
	if !r.OK() || *r.Used != 34 {
		t.Fatalf("reading=%#v, want successful 34%% usage", r)
	}
}

func TestDevinWithInvalidVersionDoesNotRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer srv.Close()
	for _, version := range []string{"", "9c803229faa4", "3000.11"} {
		r := DevinWith(srv.URL, "synthetic-key", version, time.Now())
		if r.OK() || !strings.Contains(strings.ToLower(r.Detail), "version") {
			t.Errorf("version %q: reading=%#v, want local version failure", version, r)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid version sent %d HTTP requests", requests)
	}
}

func TestDevinVersionParsesInstalledCLIOutputAndExitCode(t *testing.T) {
	oldPath, hadPath := os.LookupEnv("PATH")
	oldExt, hadExt := os.LookupEnv("PATHEXT")
	defer func() {
		if hadPath {
			os.Setenv("PATH", oldPath)
		} else {
			os.Unsetenv("PATH")
		}
		if hadExt {
			os.Setenv("PATHEXT", oldExt)
		} else {
			os.Unsetenv("PATHEXT")
		}
		devinVerOnce = struct {
			v   string
			ok  bool
			set bool
		}{}
	}()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "devin.cmd"), []byte("@echo off\r\necho devin 3000.11.3 (9c803229faa4)\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	os.Setenv("PATH", dir+string(os.PathListSeparator)+filepath.Join(os.Getenv("SystemRoot"), "System32"))
	os.Setenv("PATHEXT", ".CMD")
	devinVerOnce = struct {
		v   string
		ok  bool
		set bool
	}{}
	if got := devinVersion(); got != "3000.11.3" {
		t.Fatalf("devinVersion()=%q, want installed semver", got)
	}

	if err := os.WriteFile(filepath.Join(dir, "devin.cmd"), []byte("@echo off\r\necho devin 3000.11.3 (9c803229faa4)\r\nexit /b 1\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	devinVerOnce = struct {
		v   string
		ok  bool
		set bool
	}{}
	if got := devinVersion(); got != "" {
		t.Fatalf("failed CLI exited with version %q", got)
	}
}
