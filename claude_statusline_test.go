package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstallClaudeFeedPreservesOtherSettingsAndIsIdempotent(t *testing.T) {
	configDir := t.TempDir()
	supportDir := t.TempDir()
	settingsPath := filepath.Join(configDir, "settings.json")
	before := []byte(`{"env":{"keep":"value"},"statusLine":{"type":"command","command":"old hud","padding":7,"refreshInterval":13,"custom":{"x":true}},"other":[1,2]}`)
	if err := os.WriteFile(settingsPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installClaudeFeedAt(configDir, supportDir, `C:\Program Files\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	installed := readJSONObject(t, settingsPath)
	if !sameJSON(installed["env"], []byte(`{"keep":"value"}`)) {
		t.Fatalf("env changed: %s", installed["env"])
	}
	if !sameJSON(installed["other"], []byte(`[1,2]`)) {
		t.Fatalf("other changed: %s", installed["other"])
	}
	var status map[string]json.RawMessage
	if err := json.Unmarshal(installed["statusLine"], &status); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"type": `"command"`, "padding": `7`, "refreshInterval": `13`, "custom": `{"x":true}`} {
		if !sameJSON(status[key], []byte(want)) {
			t.Errorf("statusLine.%s = %s, want %s", key, status[key], want)
		}
	}
	var command string
	if err := json.Unmarshal(status["command"], &command); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "--claude-statusline") {
		t.Fatalf("installed command lacks bridge flag: %q", command)
	}
	backup, err := os.ReadFile(claudeStatuslineBackupPath(configDir, supportDir))
	if err != nil {
		t.Fatal(err)
	}
	var saved claudeStatuslineBackup
	if err := json.Unmarshal(backup, &saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Present || !bytes.Equal(saved.StatusLine, beforeStatusLine(t, before)) {
		t.Fatalf("backup did not preserve only prior statusLine: %+v", saved)
	}
	if strings.Contains(string(backup), `"env"`) || strings.Contains(string(backup), "value") {
		t.Fatalf("backup included unrelated settings: %s", backup)
	}
	firstSettings, _ := os.ReadFile(settingsPath)
	firstBackup, _ := os.ReadFile(claudeStatuslineBackupPath(configDir, supportDir))
	if err := installClaudeFeedAt(configDir, supportDir, `C:\Program Files\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	secondSettings, _ := os.ReadFile(settingsPath)
	secondBackup, _ := os.ReadFile(claudeStatuslineBackupPath(configDir, supportDir))
	if !bytes.Equal(firstSettings, secondSettings) || !bytes.Equal(firstBackup, secondBackup) {
		t.Fatal("second installation changed settings or replaced original backup")
	}
}

func TestInstallClaudeFeedRefusesInvalidSettingsWithoutChangingThem(t *testing.T) {
	for _, input := range []string{`{`, `[]`, `{"statusLine":"unexpected"}`, `{"statusLine":{"command":false}}`} {
		t.Run(input, func(t *testing.T) {
			configDir, supportDir := t.TempDir(), t.TempDir()
			path := filepath.Join(configDir, "settings.json")
			want := []byte(input)
			if err := os.WriteFile(path, want, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := installClaudeFeedAt(configDir, supportDir, `C:\agent-quota.exe`); err == nil {
				t.Fatal("expected invalid settings to be rejected")
			}
			got, _ := os.ReadFile(path)
			if !bytes.Equal(got, want) {
				t.Fatalf("settings changed on rejected install: %s", got)
			}
		})
	}
}

func TestInstallClaudeFeedProtectsExistingBridge(t *testing.T) {
	configDir, supportDir := t.TempDir(), t.TempDir()
	path := filepath.Join(configDir, "settings.json")
	want := []byte(`{"statusLine":{"type":"command","command":"\"C:/other/agent-quota.exe\" --claude-statusline","padding":4}}`)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installClaudeFeedAt(configDir, supportDir, `C:\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, want) {
		t.Fatal("an existing bridge was wrapped or replaced")
	}
	if _, err := os.Stat(claudeStatuslineBackupPath(configDir, supportDir)); !os.IsNotExist(err) {
		t.Fatalf("unexpected backup for pre-existing bridge: %v", err)
	}
}

func TestInstallClaudeFeedPreservesOrdinaryHUDWithFlagPrefix(t *testing.T) {
	configDir, supportDir := t.TempDir(), t.TempDir()
	path := filepath.Join(configDir, "settings.json")
	command := `"C:/Users/test/status-renderer.exe" --claude-statusline-theme compact`
	want := []byte(`{"statusLine":{"type":"command","command":"\"C:/Users/test/status-renderer.exe\" --claude-statusline-theme compact","padding":5}}`)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if isClaudeStatuslineBridge(command, `C:\agent-quota.exe`) {
		t.Fatal("flag prefix was mistaken for the Agent Quota bridge")
	}
	if err := installClaudeFeedAt(configDir, supportDir, `C:\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	if got := originalClaudeStatusline(configDir, supportDir); got != command {
		t.Fatalf("saved renderer = %q, want %q", got, command)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(want, &root); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(claudeStatuslineBackupPath(configDir, supportDir))
	if err != nil {
		t.Fatal(err)
	}
	var saved claudeStatuslineBackup
	if err := json.Unmarshal(backup, &saved); err != nil || !saved.Present {
		t.Fatalf("user renderer wasn't backed up: %s (%v)", backup, err)
	}
}

func TestInstallClaudeFeedRefreshesBackupAfterUserChangesHUD(t *testing.T) {
	configDir, supportDir := t.TempDir(), t.TempDir()
	path := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"statusLine":{"type":"command","command":"first hud","padding":3}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installClaudeFeedAt(configDir, supportDir, `C:\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	changed := []byte(`{"statusLine":{"type":"command","command":"user's replacement hud","padding":9}}`)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := installClaudeFeedAt(configDir, supportDir, `C:\agent-quota.exe`); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(claudeStatuslineBackupPath(configDir, supportDir))
	if err != nil {
		t.Fatal(err)
	}
	var saved claudeStatuslineBackup
	if err := json.Unmarshal(backup, &saved); err != nil {
		t.Fatal(err)
	}
	var status map[string]json.RawMessage
	if err := json.Unmarshal(saved.StatusLine, &status); err != nil {
		t.Fatal(err)
	}
	var command string
	if err := json.Unmarshal(status["command"], &command); err != nil {
		t.Fatal(err)
	}
	if command != "user's replacement hud" {
		t.Fatalf("saved HUD = %q, want current user HUD", command)
	}
}

func TestClaudeStatuslineRendererForwardsInputAndOutput(t *testing.T) {
	t.Setenv("AGENTQUOTA_TEST_RENDERER", "1")
	helper := filepath.Join(t.TempDir(), "renderer path with spaces.exe")
	copyExecutable(t, os.Args[0], helper)
	command := rendererTestCommand(helper, claudeGitBashPath() != "")
	input := []byte("{\"model\":\"test\",\"nested\":[1,true]}\n")
	var stdout, stderr bytes.Buffer
	if err := runClaudeStatuslineCommand(command, input, &stdout, &stderr); err != nil {
		t.Fatalf("renderer failed: %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), input) {
		t.Fatalf("stdout = %q, want exact input %q", stdout.Bytes(), input)
	}
	if stderr.String() != "renderer stderr\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestClaudeGitBashDiscoveryUsesGitInstallOnly(t *testing.T) {
	root := t.TempDir()
	gitExe := filepath.Join(root, "Git", "cmd", "git.exe")
	bashExe := filepath.Join(root, "Git", "bin", "bash.exe")
	makeTestFile(t, gitExe)
	makeTestFile(t, bashExe)
	if got := gitBashBesideGit(gitExe); got != bashExe {
		t.Fatalf("Git Bash = %q, want %q", got, bashExe)
	}
	if got := gitBashInInstall(filepath.Join(root, "Git")); got != bashExe {
		t.Fatalf("Git install Bash = %q, want %q", got, bashExe)
	}

	wslBash := filepath.Join(root, "Windows", "System32", "bash.exe")
	makeTestFile(t, wslBash)
	if got := gitBashBesideGit(wslBash); got != "" {
		t.Fatalf("accepted non-Git bash.exe: %q", got)
	}
	if got := gitBashInInstall(filepath.Join(root, "Windows")); got != "" {
		t.Fatalf("accepted Bash without a Git executable: %q", got)
	}
}

func TestClaudeBridgeCommandSupportsPowerShellFallback(t *testing.T) {
	exe := `C:\Program Files\Agent Quota\agent-quota.exe`
	if got, want := claudeBridgeCommand(exe, false), `& "C:/Program Files/Agent Quota/agent-quota.exe" --claude-statusline`; got != want {
		t.Fatalf("PowerShell bridge command = %q, want %q", got, want)
	}
	if got, want := claudeBridgeCommand(exe, true), `"C:/Program Files/Agent Quota/agent-quota.exe" --claude-statusline`; got != want {
		t.Fatalf("Git Bash bridge command = %q, want %q", got, want)
	}
}

func rendererTestCommand(helper string, gitBash bool) string {
	if gitBash {
		return `if [[ "$AGENTQUOTA_TEST_RENDERER" == "1" ]]; then "` + filepath.ToSlash(helper) + `" -test.run=^TestRendererHelper$; else exit 9; fi`
	}
	quoted := strings.ReplaceAll(filepath.ToSlash(helper), "'", "''")
	return `if ($env:AGENTQUOTA_TEST_RENDERER -eq '1') { & '` + quoted + `' -test.run=^TestRendererHelper$ } else { exit 9 }`
}

func makeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyExecutable(t *testing.T, source, destination string) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRendererHelper(t *testing.T) {
	if os.Getenv("AGENTQUOTA_TEST_RENDERER") != "1" {
		return
	}
	_, _ = io.Copy(os.Stdout, os.Stdin)
	_, _ = io.WriteString(os.Stderr, "renderer stderr\n")
	os.Exit(0)
}

func readJSONObject(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func beforeStatusLine(t *testing.T, before []byte) []byte {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(before, &root); err != nil {
		t.Fatal(err)
	}
	return root["statusLine"]
}

func sameJSON(got, want []byte) bool {
	var a, b any
	return json.Unmarshal(got, &a) == nil && json.Unmarshal(want, &b) == nil && reflect.DeepEqual(a, b)
}
