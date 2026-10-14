package win

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "AgentQuota"
)

// AutostartEnabled reports whether Start with Windows is on.
func AutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, typ, err := k.GetStringValue(runValue)
	return err == nil && (typ == registry.SZ || typ == registry.EXPAND_SZ) && v != ""
}

// SetAutostart writes/removes the Run value.
func SetAutostart(on bool) error {
	if !on {
		if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE); err == nil {
			k.DeleteValue(runValue)
			k.Close()
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runValue, `"`+exe+`"`)
}
