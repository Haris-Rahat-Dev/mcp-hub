package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
	"github.com/harisbinrahat/mcp-hub/internal/launch"
)

func runLaunch(args []string) error {
	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	noSync := fs.Bool("no-sync", false, "skip sync before launching")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub launch [--config <path>] [--no-sync] <client-id>")
	}
	clientID := fs.Arg(0)

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	// Step 1: Ensure hub is running.
	if err := ensureHubRunning(cfg, *configPath); err != nil {
		return fmt.Errorf("hub is not reachable: %w\nRun 'mcp-hub start' to start the hub.", err)
	}

	// Step 2: Sync client config unless --no-sync.
	if !*noSync {
		res, err := generate.Sync(cfg, clientID, false)
		if err != nil {
			return fmt.Errorf("sync %s: %w", clientID, err)
		}
		if res.DriftWarn {
			fmt.Fprintf(os.Stderr, "Warning: drift detected for %s — managed entries were overwritten\n", clientID)
		}
	}

	// Step 3: Resolve manifest and launch the app.
	clients := generate.ResolveClients(cfg)
	manifest, ok := clients[clientID]
	if !ok {
		return fmt.Errorf("unknown client %q", clientID)
	}

	return launchClient(clientID, manifest)
}

// ensureHubRunning checks if the hub is reachable; if not, starts it and waits.
func ensureHubRunning(cfg config.Config, configPath string) error {
	addr := "http://" + cfg.Hub.ListenAddress() + "/status"
	if isHubReachable(addr) {
		return nil
	}

	// Try to start the hub.
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	startArgs := []string{"start"}
	if configPath != "" {
		startArgs = append(startArgs, "--config", configPath)
	}
	cmd := exec.Command(exePath, startArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start hub: %w", err)
	}

	// Wait up to 5 seconds for the hub to become reachable.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if isHubReachable(addr) {
			return nil
		}
	}
	return fmt.Errorf("hub did not become reachable within 5 seconds")
}

func isHubReachable(addr string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(addr)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var snap struct{ Version string }
	_ = json.NewDecoder(resp.Body).Decode(&snap)
	return resp.StatusCode == http.StatusOK
}

func launchClient(clientID string, manifest config.ClientManifest) error {
	if err := launch.Manifest(manifest); err != nil {
		if clientID != "" {
			return fmt.Errorf("client %q: %w", clientID, err)
		}
		return err
	}
	return nil
}
