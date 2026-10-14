package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

func known(id *windows.KNOWNFOLDERID) string {
	p, err := windows.KnownFolderPath(id, 0)
	if err != nil {
		return ""
	}
	return p
}

// AppData is %APPDATA% (roaming). "" when Windows has none.
func AppData() string { return known(windows.FOLDERID_RoamingAppData) }

// ConfigDir: only Linux keeps other apps' settings in one config folder.
func ConfigDir() string { return "" }

// FullPath is Path.GetFullPath.
func FullPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

var supportOnce = sync.OnceValue(func() string {
	if over := os.Getenv("AGENTQUOTA_DATA_DIR"); over != "" {
		_ = os.MkdirAll(over, 0o755)
		return over
	}
	base := AppData()
	if base == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "agent-quota")
	_ = os.MkdirAll(dir, 0o755)
	return dir
})

// Support is %APPDATA%\agent-quota, made once.
func Support() string { return supportOnce() }

func SettingsFile() string { return filepath.Join(Support(), "settings.json") }
func LogFile() string      { return filepath.Join(Support(), "failure.log") }

var logGate sync.Mutex

// Logf is one line to stderr and to failure.log. Logging never takes the app down.
func Logf(format string, a ...any) {
	stamp := fmt.Sprintf(format, a...)
	fmt.Fprintln(os.Stderr, "agent-quota: "+stamp)
	logGate.Lock()
	defer logGate.Unlock()
	if f, err := os.OpenFile(LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(f, stamp)
		f.Close()
	}
}
