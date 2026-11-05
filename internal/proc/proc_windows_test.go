package proc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBatchArgHelper(t *testing.T) {
	if os.Getenv("AGENTQUOTA_BATCH_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			if err := json.NewEncoder(os.Stdout).Encode(os.Args[i+1:]); err != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestHiddenBatchArgumentsRoundTrip(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "batch tools & data")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{},
		{"plain", "two words", ""},
		{`say "hello"`, `C:\path with spaces\`},
		{"literal&echo.AGENTQUOTA_UNEXPECTED", "paren(value)^tail"},
		{"%AGENTQUOTA_BATCH_EXPAND%", "!AGENTQUOTA_BATCH_EXPAND!"},
	}
	for _, ext := range []string{".CMD", ".bat"} {
		script := filepath.Join(dir, "probe"+ext)
		// Forward through a real shim, as CLI .cmd entry points do, so the
		// assertion observes the final executable's argv rather than a string.
		body := "@echo off\r\n\"" + exe + "\" -test.run=TestBatchArgHelper -- %*\r\n"
		if err := os.WriteFile(script, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		for _, args := range cases {
			t.Run(ext+strings.Join(args, ";"), func(t *testing.T) {
				cmd := Hidden(script, args...)
				cmd.Dir = dir
				cmd.Env = append(cmd.Env, "AGENTQUOTA_BATCH_HELPER=1", "AGENTQUOTA_BATCH_EXPAND=expanded")
				if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
					t.Fatal("batch command can create a console window")
				}
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("batch launch failed: %v\n%s", err, out)
				}
				var got []string
				if err := json.Unmarshal(out, &got); err != nil {
					t.Fatalf("batch output is not argv JSON: %v\n%s", err, out)
				}
				if !reflect.DeepEqual(got, args) {
					t.Fatalf("batch changed arguments: got %q, want %q", got, args)
				}
			})
		}
	}
}

func TestHiddenBatchRejectsInvalidArguments(t *testing.T) {
	for _, arg := range []string{"line\rbreak", "line\nbreak", "nul\x00byte"} {
		if cmd := Hidden(`C:\tools\probe.cmd`, arg); cmd.Err == nil {
			t.Errorf("accepted invalid batch argument %q", arg)
		}
	}
}

func TestBatCommandRejectsInvalidScript(t *testing.T) {
	for _, script := range []string{`C:\bad"name.cmd`, `C:\tools\`, "C:\\bad\x00name.cmd", "C:\\bad\nname.cmd", "C:\\bad\rname.cmd"} {
		if cmd := batCommand(script, nil); cmd.Err == nil {
			t.Errorf("accepted invalid batch path %q", script)
		}
	}
}
