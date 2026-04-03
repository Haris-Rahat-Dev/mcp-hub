package launch

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/harisbinrahat/mcp-hub/internal/config"
)

// Manifest opens a GUI app (macOS) or runs a CLI executable using the client manifest.
func Manifest(manifest config.ClientManifest) error {
	switch manifest.Kind {
	case "gui":
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("GUI launch is only supported on macOS")
		}
		if manifest.BundleID != "" {
			return exec.Command("open", "-b", manifest.BundleID).Run()
		}
		if manifest.DisplayName != "" {
			return exec.Command("open", "-a", manifest.DisplayName).Run()
		}
		return fmt.Errorf("manifest has no bundle_id or display_name to launch")
	case "cli":
		if manifest.Executable == "" {
			return fmt.Errorf("manifest has no executable configured")
		}
		cmd := exec.Command(manifest.Executable)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("unknown client kind %q", manifest.Kind)
	}
}
