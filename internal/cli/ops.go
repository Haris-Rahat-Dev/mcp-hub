package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
	"github.com/harisbinrahat/mcp-hub/internal/importer"
)

func runInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		plistPath, err := launchAgentPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
			return err
		}

		content := launchdPlist(exePath, cfg.Path)
		if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
			return err
		}

		_ = exec.Command("launchctl", "unload", plistPath).Run()
		if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
			return err
		}
		fmt.Printf("Installed launchd service: %s\n", plistPath)

		// Generate CLI shims and .app wrappers for all known clients.
		if err := generateClientArtifacts(cfg, exePath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: client artifact generation: %v\n", err)
		}
		return nil
	case "linux":
		unitPath, err := systemdUserUnitPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
			return err
		}
		content := systemdUnit(exePath, cfg.Path)
		if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
			return err
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		_ = exec.Command("systemctl", "--user", "enable", "--now", "mcp-hub.service").Run()
		fmt.Printf("Installed systemd user service: %s\n", unitPath)
		return nil
	case "windows":
		startupPath, err := windowsStartupScriptPath()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(startupPath), 0o755); err != nil {
			return err
		}
		content := windowsStartupScript(exePath, cfg.Path)
		if err := os.WriteFile(startupPath, []byte(content), 0o644); err != nil {
			return err
		}
		fmt.Printf("Installed Windows startup entry: %s\n", startupPath)
		return nil
	default:
		return fmt.Errorf("install is currently implemented for macOS launchd, Linux systemd user services, and Windows startup entries")
	}
}

func runUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		plistPath, err := launchAgentPath()
		if err != nil {
			return err
		}
		_ = exec.Command("launchctl", "unload", plistPath).Run()
		if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("Uninstalled launchd service: %s\n", plistPath)
		return nil
	case "linux":
		unitPath, err := systemdUserUnitPath()
		if err != nil {
			return err
		}
		_ = exec.Command("systemctl", "--user", "disable", "--now", "mcp-hub.service").Run()
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("Uninstalled systemd user service: %s\n", unitPath)
		return nil
	case "windows":
		startupPath, err := windowsStartupScriptPath()
		if err != nil {
			return err
		}
		if err := os.Remove(startupPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Printf("Uninstalled Windows startup entry: %s\n", startupPath)
		return nil
	default:
		return fmt.Errorf("uninstall is currently implemented for macOS launchd, Linux systemd user services, and Windows startup entries")
	}
}

func runLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	follow := fs.Bool("follow", false, "follow log output")
	lines := fs.Int("n", 80, "number of lines to show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	printTail := func() error {
		data, err := os.ReadFile(cfg.Hub.LogFile)
		if err != nil {
			return err
		}
		all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		if len(all) > *lines {
			all = all[len(all)-*lines:]
		}
		fmt.Println(strings.Join(all, "\n"))
		return nil
	}

	if err := printTail(); err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	lastSize := int64(0)
	if info, err := os.Stat(cfg.Hub.LogFile); err == nil {
		lastSize = info.Size()
	}
	for {
		time.Sleep(1 * time.Second)
		info, err := os.Stat(cfg.Hub.LogFile)
		if err != nil {
			continue
		}
		if info.Size() <= lastSize {
			continue
		}
		f, err := os.Open(cfg.Hub.LogFile)
		if err != nil {
			continue
		}
		if _, err := f.Seek(lastSize, io.SeekStart); err == nil {
			_, _ = io.Copy(os.Stdout, f)
		}
		_ = f.Close()
		lastSize = info.Size()
	}
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to mcp-hub config file")
	clientID := fs.String("client", "", "client id to store unmappable fields as passthrough")
	extractSecrets := fs.Bool("extract-secrets", false, "replace literal secret values with ${ENV_VAR} references (opt-in)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub import <existing-mcp-json> [--client <id>] [--extract-secrets]")
	}
	srcPath := fs.Arg(0)
	_ = extractSecrets // secret extraction is opt-in; placeholder for future implementation
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	imported, err := importer.ImportMCPJSON(*configPath, srcBytes, *clientID)
	if err != nil {
		return err
	}
	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}
	fmt.Printf("Imported %d servers into %s\n", imported, cfg.Path)
	return nil
}

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.mcp-hub.plist"), nil
}

func systemdUserUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", "mcp-hub.service"), nil
}

func windowsStartupScriptPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return windowsStartupScriptPathFor(os.Getenv("APPDATA"), home), nil
}

func windowsStartupScriptPathFor(appData, home string) string {
	base := appData
	if base == "" {
		base = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "mcp-hub.cmd")
}

func windowsStartupScript(executablePath, configPath string) string {
	return fmt.Sprintf(`@echo off
REM mcp-hub startup entry generated by mcp-hub install
start "" /min "%s" start --config "%s"
`, executablePath, configPath)
}

func launchdPlist(executablePath, configPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.mcp-hub</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>start</string>
    <string>--config</string>
    <string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/mcp-hub.out.log</string>
  <key>StandardErrorPath</key><string>/tmp/mcp-hub.err.log</string>
</dict>
</plist>
`, executablePath, configPath)
}

// generateClientArtifacts creates CLI shims and .app wrappers for all clients
// with non-empty ConfigPath in the resolved manifest set.
func generateClientArtifacts(cfg config.Config, mcpHubExe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	shimDir := filepath.Join(home, ".mcp-hub", "bin")
	appDir := filepath.Join(home, "Applications", "MCP Hub Apps")

	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}

	for clientID, manifest := range generate.ResolveClients(cfg) {
		if strings.TrimSpace(manifest.ConfigPath) == "" {
			continue
		}
		switch manifest.Kind {
		case "cli":
			if manifest.Executable == "" {
				continue
			}
			if err := generateCLIShim(shimDir, clientID, manifest.Executable, mcpHubExe, cfg.Path); err != nil {
				fmt.Fprintf(os.Stderr, "  shim %s: %v\n", clientID, err)
			} else {
				fmt.Printf("  Generated CLI shim: %s\n", filepath.Join(shimDir, clientID))
			}
		case "gui":
			if manifest.DisplayName == "" {
				continue
			}
			if err := generateAppWrapper(appDir, clientID, manifest, mcpHubExe, cfg.Path); err != nil {
				fmt.Fprintf(os.Stderr, "  .app %s: %v\n", clientID, err)
			} else {
				fmt.Printf("  Generated .app wrapper: %s\n", filepath.Join(appDir, manifest.DisplayName+" (MCP Hub).app"))
			}
		}
	}

	fmt.Printf("\nAdd CLI shims to PATH:\n  export PATH=\"%s:$PATH\"\n", shimDir)
	return nil
}

func generateCLIShim(shimDir, clientID, executable, mcpHubExe, configPath string) error {
	script := fmt.Sprintf(`#!/bin/sh
# mcp-hub CLI shim for %s — generated by mcp-hub install
%s launch --config %q %s || { echo "mcp-hub: launch failed" >&2; exit 1; }
exec %q "$@"
`, clientID, mcpHubExe, configPath, clientID, executable)

	dest := filepath.Join(shimDir, clientID)
	if err := os.WriteFile(dest, []byte(script), 0o755); err != nil {
		return err
	}
	return nil
}

func generateAppWrapper(appDir, clientID string, manifest config.ClientManifest, mcpHubExe, configPath string) error {
	appName := manifest.DisplayName + " (MCP Hub).app"
	appPath := filepath.Join(appDir, appName)
	contentsDir := filepath.Join(appPath, "Contents")
	macOSDir := filepath.Join(contentsDir, "MacOS")

	for _, dir := range []string{contentsDir, macOSDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	// Info.plist
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>com.mcp-hub.%s</string>
  <key>CFBundleDisplayName</key><string>%s (MCP Hub)</string>
  <key>CFBundleName</key><string>%s</string>
  <key>CFBundleExecutable</key><string>launcher</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleVersion</key><string>1.0</string>
</dict>
</plist>
`, clientID, manifest.DisplayName, manifest.DisplayName)

	if err := os.WriteFile(filepath.Join(contentsDir, "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}

	// Launcher script
	var launchCmd string
	if manifest.BundleID != "" {
		launchCmd = fmt.Sprintf(`open -b %q`, manifest.BundleID)
	} else {
		launchCmd = fmt.Sprintf(`open -a %q`, manifest.DisplayName)
	}

	launcher := fmt.Sprintf(`#!/bin/sh
# mcp-hub .app launcher for %s — generated by mcp-hub install
if ! %s launch --config %q %s; then
  osascript -e 'display dialog "mcp-hub launch failed for %s. Check logs with: mcp-hub logs" buttons {"OK"} default button "OK" with icon stop' 2>/dev/null || true
  exit 1
fi
%s
`, clientID, mcpHubExe, configPath, clientID, manifest.DisplayName, launchCmd)

	launcherPath := filepath.Join(macOSDir, "launcher")
	if err := os.WriteFile(launcherPath, []byte(launcher), 0o755); err != nil {
		return err
	}
	return nil
}

// applyPassthrough adds unmappable fields from obj into sc.Passthrough[clientID]
// when clientID is non-empty. knownKeys lists the keys already handled.
func applyPassthrough(sc config.ServerConfig, obj map[string]any, clientID string, knownKeys []string) config.ServerConfig {
	if clientID == "" {
		return sc
	}
	known := make(map[string]bool, len(knownKeys))
	for _, k := range knownKeys {
		known[k] = true
	}
	extras := map[string]any{}
	for k, v := range obj {
		if !known[k] {
			extras[k] = v
		}
	}
	if len(extras) > 0 {
		if sc.Passthrough == nil {
			sc.Passthrough = map[string]map[string]any{}
		}
		sc.Passthrough[clientID] = extras
	}
	return sc
}

// parseStringMap converts a JSON object value (map[string]any) into a
// map[string]string, ignoring non-string values.
func parseStringMap(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseBoolPtr returns a *bool from the first truthy-typed value found among
// the provided candidates. Accepts bool and float64 (JSON number) types.
func parseBoolPtr(candidates ...any) *bool {
	for _, v := range candidates {
		switch val := v.(type) {
		case bool:
			b := val
			return &b
		case float64:
			b := val != 0
			return &b
		}
	}
	return nil
}

// parseInt returns the first non-zero integer found among the candidates.
func parseInt(candidates ...any) int {
	for _, v := range candidates {
		switch val := v.(type) {
		case float64:
			if i := int(val); i > 0 {
				return i
			}
		case int:
			if val > 0 {
				return val
			}
		}
	}
	return 0
}

func systemdUnit(executablePath, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=mcp-hub daemon
After=network.target

[Service]
Type=simple
ExecStart=%s start --config %s
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
`, executablePath, configPath)
}
