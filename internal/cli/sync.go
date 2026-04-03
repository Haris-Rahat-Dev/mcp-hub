package cli

import (
	"flag"
	"fmt"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
)

func runSync(args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	dryRun := fs.Bool("dry-run", false, "report changes without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	if fs.NArg() == 1 {
		clientID := fs.Arg(0)
		res, err := generate.Sync(cfg, clientID, *dryRun)
		if err != nil {
			return err
		}
		printSyncResult(res, *dryRun)
		return nil
	}

	results, err := generate.SyncAll(cfg, *dryRun)
	if err != nil {
		return err
	}
	for _, res := range results {
		printSyncResult(res, *dryRun)
	}
	return nil
}

func printSyncResult(res generate.SyncResult, dryRun bool) {
	if res.Path == "" {
		fmt.Printf("  %-20s skipped (no config path)\n", res.ClientID)
		return
	}
	if !res.Written && !dryRun {
		fmt.Printf("  %-20s not detected (%s)\n", res.ClientID, res.Path)
		return
	}
	action := "synced"
	if dryRun {
		action = "would sync"
	}
	drift := ""
	if res.DriftWarn {
		drift = " [DRIFT DETECTED — overwritten]"
	}
	fmt.Printf("  %-20s %s → %s (%d servers, %d preserved)%s\n",
		res.ClientID, action, res.Path, res.ServerCount, res.Preserved, drift)
}
