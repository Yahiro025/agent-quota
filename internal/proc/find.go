package proc

import (
	"os"
	"path/filepath"
	"runtime"
)

// Find is a command on PATH, else Kiro's install folder on Windows; "" when neither.
func Find(name string) string {
	if p := OnPath(name); p != "" {
		return p
	}
	if name == "kiro-cli" {
		if p := kiroInstalled(); p != "" {
			return p
		}
	}
	return ""
}

func kiroInstalled() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		if p := filepath.Join(root, "Kiro-Cli", "kiro-cli.exe"); isFile(p) {
			return p
		}
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
