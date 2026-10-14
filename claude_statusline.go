package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"agent-quota/internal/core"
	"agent-quota/internal/proc"
	"agent-quota/internal/win"
	"agent-quota/quota"
)

const (
	claudeStatuslineMaxInput    = 1 << 20
	claudeStatuslineMaxSettings = 1 << 20
	claudeStatuslineMaxRun      = 10 * time.Second
	claudeStatuslineBackupName  = "claude-statusline.json"
	menuClaudeSnapshot          = 6
	claudeSnapshotWait          = 150 * time.Millisecond
)

type claudeStatuslineBackup struct {
	Scope      string          `json:"scope"`
	Present    bool            `json:"present"`
	StatusLine json.RawMessage `json:"statusLine,omitempty"`
}

// installClaudeFeedAt is parameterized so tests never open the user's Claude config.
func installClaudeFeedAt(configDir, supportDir, exe string) error {
	settingsPath := filepath.Join(configDir, "settings.json")
	root, err := readClaudeSettings(settingsPath, true)
	if err != nil {
		return err
	}
	status, hasStatus := root["statusLine"]
	var statusFields map[string]json.RawMessage
	if hasStatus {
		if err := json.Unmarshal(status, &statusFields); err != nil || statusFields == nil {
			return errors.New("unexpected statusLine setting")
		}
		if typeRaw, ok := statusFields["type"]; ok {
			var typ string
			if json.Unmarshal(typeRaw, &typ) != nil || typ != "command" {
				return errors.New("unexpected statusLine type")
			}
		}
		if commandRaw, ok := statusFields["command"]; ok {
			var command string
			if json.Unmarshal(commandRaw, &command) != nil {
				return errors.New("unexpected statusLine command")
			}
			if isClaudeStatuslineBridge(command, exe) {
				return nil
			}
		}
	}

	scope, err := filepath.Abs(configDir)
	if err != nil {
		return errors.New("could not resolve Claude config scope")
	}
	backup := claudeStatuslineBackup{Scope: scope, Present: hasStatus}
	if hasStatus {
		backup.StatusLine = append(json.RawMessage(nil), status...)
	}
	encodedBackup, err := json.Marshal(backup)
	if err != nil {
		return errors.New("could not save existing Claude statusLine")
	}
	if err := writeAtomic(claudeStatuslineBackupPath(configDir, supportDir), encodedBackup, 0o600); err != nil {
		return errors.New("could not save existing Claude statusLine")
	}

	if statusFields == nil {
		statusFields = map[string]json.RawMessage{}
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return errors.New("could not resolve Agent Quota executable")
	}
	command := claudeBridgeCommand(exe, claudeGitBashPath() != "")
	commandJSON, _ := json.Marshal(command)
	statusFields["type"] = json.RawMessage(`"command"`)
	statusFields["command"] = commandJSON
	encodedStatus, err := json.Marshal(statusFields)
	if err != nil {
		return errors.New("could not update Claude statusLine")
	}
	root["statusLine"] = encodedStatus
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return errors.New("could not update Claude settings")
	}
	return writeAtomic(settingsPath, encoded, 0o600)
}

func claudeStatuslineBackupPath(configDir, supportDir string) string {
	scope, err := filepath.Abs(configDir)
	if err != nil {
		scope = filepath.Clean(configDir)
	}
	if filepath.Separator == '\\' {
		scope = strings.ToLower(scope)
	}
	digest := sha256.Sum256([]byte(scope))
	return filepath.Join(supportDir, claudeStatuslineBackupName+"-"+hex.EncodeToString(digest[:]))
}

func readClaudeSettings(path string, missingIsEmpty bool) (map[string]json.RawMessage, error) {
	b, err := readBoundedFile(path, claudeStatuslineMaxSettings)
	if errors.Is(err, os.ErrNotExist) && missingIsEmpty {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, errors.New("could not read Claude settings")
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(b, &root) != nil || root == nil {
		return nil, errors.New("invalid Claude settings")
	}
	return root, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("settings exceed size limit")
	}
	return b, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agentquota-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func isClaudeStatuslineBridge(command, expectedExe string) bool {
	hasFlag, hasAgentQuota := false, false
	expectedExe, _ = filepath.Abs(expectedExe)
	for _, token := range commandTokens(command) {
		if token == "--claude-statusline" {
			hasFlag = true
		}
		base := strings.ToLower(path.Base(strings.ReplaceAll(token, `\`, "/")))
		if base == "agentquota.exe" || base == "agent-quota.exe" {
			hasAgentQuota = true
		}
		if expectedExe != "" {
			candidate, err := filepath.Abs(token)
			if err == nil && strings.EqualFold(filepath.Clean(candidate), filepath.Clean(expectedExe)) {
				hasAgentQuota = true
			}
		}
	}
	return hasFlag && hasAgentQuota
}

func commandTokens(command string) []string {
	var tokens []string
	var token strings.Builder
	var quote rune
	flush := func() {
		if token.Len() != 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}
	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == 0 {
			if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
				flush()
				continue
			}
			if r == '"' || r == '\'' {
				quote = r
				continue
			}
			token.WriteRune(r)
			continue
		}
		if r == quote {
			if quote == '\'' && i+1 < len(runes) && runes[i+1] == quote {
				token.WriteRune(r)
				i++
			} else {
				quote = 0
			}
			continue
		}
		if r == '\\' && quote == '"' && i+1 < len(runes) && runes[i+1] == '"' {
			token.WriteRune('"')
			i++
			continue
		}
		token.WriteRune(r)
	}
	flush()
	return tokens
}

func originalClaudeStatusline(configDir, supportDir string) string {
	root, err := readClaudeSettings(filepath.Join(configDir, "settings.json"), false)
	if err != nil {
		return ""
	}
	var current struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(root["statusLine"], &current) != nil {
		return ""
	}
	exe, _ := os.Executable()
	if !isClaudeStatuslineBridge(current.Command, exe) {
		return current.Command
	}
	b, err := readBoundedFile(claudeStatuslineBackupPath(configDir, supportDir), claudeStatuslineMaxSettings)
	if err != nil {
		return ""
	}
	var backup claudeStatuslineBackup
	if json.Unmarshal(b, &backup) != nil || !backup.Present {
		return ""
	}
	scope, err := filepath.Abs(configDir)
	if err != nil || !strings.EqualFold(filepath.Clean(scope), filepath.Clean(backup.Scope)) {
		return ""
	}
	var saved struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(backup.StatusLine, &saved) != nil {
		return ""
	}
	return saved.Command
}

func handleClaudeStatusline() {
	input, err := io.ReadAll(io.LimitReader(os.Stdin, claudeStatuslineMaxInput+1))
	if err != nil || len(input) > claudeStatuslineMaxInput {
		return
	}
	captureDone := make(chan struct{}, 1)
	go func() {
		if quota.CaptureClaudeSnapshot(input, time.Now()) {
			win.NotifyExisting("AgentQuota", menuClaudeSnapshot)
		}
		captureDone <- struct{}{}
	}()
	command := originalClaudeStatusline(quota.ClaudeHome(), core.Support())
	if command == "" {
		waitForClaudeSnapshot(captureDone)
		return
	}
	_ = runClaudeStatuslineCommand(command, input, os.Stdout, os.Stderr)
	waitForClaudeSnapshot(captureDone)
}

func waitForClaudeSnapshot(done <-chan struct{}) {
	timer := time.NewTimer(claudeSnapshotWait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// runClaudeStatuslineCommand gives the user's existing renderer the original JSON and
// relays both streams byte-for-byte. The job object bounds child lifetime on timeout.
func runClaudeStatuslineCommand(command string, input []byte, stdout, stderr io.Writer) error {
	if command == "" || len(input) > claudeStatuslineMaxInput {
		return errors.New("invalid statusLine invocation")
	}
	var cmd *exec.Cmd
	if bash := claudeGitBashPath(); bash != "" {
		cmd = proc.Hidden(bash, "-c", command)
	} else {
		cmd = proc.Hidden("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	group, err := proc.Spawn(cmd)
	if err != nil {
		return errors.New("could not start statusLine renderer")
	}
	closed := false
	closeGroup := func() {
		if !closed {
			group.Close()
			closed = true
		}
	}
	defer closeGroup()
	in, out, errOut := group.TakePipes()
	if in == nil || out == nil || errOut == nil {
		return errors.New("could not connect statusLine renderer")
	}
	copyDone := make(chan error, 2)
	go func() { _, e := io.Copy(stdout, out); out.Close(); copyDone <- e }()
	go func() { _, e := io.Copy(stderr, errOut); errOut.Close(); copyDone <- e }()
	inputDone := make(chan error, 1)
	go func() {
		_, writeErr := in.Write(input)
		closeErr := in.Close()
		if writeErr != nil {
			inputDone <- errors.New("could not send statusLine input")
		} else if closeErr != nil {
			inputDone <- errors.New("could not finish statusLine input")
		} else {
			inputDone <- nil
		}
	}()
	waitDone := make(chan bool, 1)
	go func() {
		_, exited := group.WaitTimeout(claudeStatuslineMaxRun)
		waitDone <- exited
	}()
	timer := time.NewTimer(claudeStatuslineMaxRun)
	defer timer.Stop()
	inputFinished, processFinished := false, false
	copyFinished := 0
	var inputErr, copyErr error
	for !inputFinished || !processFinished || copyFinished != 2 {
		select {
		case <-timer.C:
			closeGroup()
			in.Close()
			out.Close()
			errOut.Close()
			return errors.New("statusLine renderer timed out")
		case exited := <-waitDone:
			processFinished = true
			if !exited {
				closeGroup()
				in.Close()
				out.Close()
				errOut.Close()
				return errors.New("statusLine renderer timed out")
			}
			waitDone = nil
		case err := <-inputDone:
			inputFinished, inputErr = true, err
			inputDone = nil
		case err := <-copyDone:
			copyFinished++
			if err != nil && copyErr == nil {
				copyErr = err
			}
		}
	}
	closeGroup()
	if inputErr != nil {
		return inputErr
	}
	if copyErr != nil {
		return errors.New("could not relay statusLine output")
	}
	return nil
}

func claudeGitBashPath() string {
	if configured := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); configured != "" {
		if isFile(configured) {
			return configured
		}
		return ""
	}
	for _, root := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Git"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git"),
	} {
		if candidate := gitBashInInstall(root); candidate != "" {
			return candidate
		}
	}
	gitExe, err := exec.LookPath("git.exe")
	if err != nil {
		return ""
	}
	return gitBashBesideGit(gitExe)
}

func gitBashBesideGit(gitExe string) string {
	gitDir := filepath.Dir(gitExe)
	if !strings.EqualFold(filepath.Base(gitDir), "cmd") && !strings.EqualFold(filepath.Base(gitDir), "bin") {
		return ""
	}
	return gitBashInInstall(filepath.Dir(gitDir))
}

func gitBashInInstall(root string) string {
	if root == "" || (!isFile(filepath.Join(root, "cmd", "git.exe")) && !isFile(filepath.Join(root, "bin", "git.exe"))) {
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(root, "bin", "bash.exe"),
		filepath.Join(root, "usr", "bin", "bash.exe"),
	} {
		if isFile(candidate) {
			return candidate
		}
	}
	return ""
}

func claudeBridgeCommand(exe string, hasGitBash bool) string {
	quotedExe := `"` + filepath.ToSlash(exe) + `"`
	if hasGitBash {
		return quotedExe + " --claude-statusline"
	}
	return "& " + quotedExe + " --claude-statusline"
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
