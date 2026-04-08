package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsStartupScriptPathForUsesAppData(t *testing.T) {
	got := windowsStartupScriptPathFor(`C:\Users\alice\AppData\Roaming`, `C:\Users\alice`)
	want := filepath.Join(`C:\Users\alice\AppData\Roaming`, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "mcp-hub.cmd")
	if got != want {
		t.Fatalf("windowsStartupScriptPathFor() = %q, want %q", got, want)
	}
}

func TestWindowsStartupScriptPathForFallsBackToHome(t *testing.T) {
	got := windowsStartupScriptPathFor("", `C:\Users\alice`)
	want := filepath.Join(`C:\Users\alice`, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "mcp-hub.cmd")
	if got != want {
		t.Fatalf("windowsStartupScriptPathFor() = %q, want %q", got, want)
	}
}

func TestWindowsStartupScriptIncludesCommandAndConfig(t *testing.T) {
	script := windowsStartupScript(`C:\Tools\mcp-hub.exe`, `C:\Users\alice\AppData\Roaming\mcp-hub\config.yaml`)
	if !strings.Contains(script, `start "" /min "C:\Tools\mcp-hub.exe" start --config "C:\Users\alice\AppData\Roaming\mcp-hub\config.yaml"`) {
		t.Fatalf("startup script missing expected command, got:\n%s", script)
	}
}
