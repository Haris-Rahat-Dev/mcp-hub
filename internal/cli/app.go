package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/harisbinrahat/mcp-hub/internal/admin"
	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
	"github.com/harisbinrahat/mcp-hub/internal/hub"
	"github.com/harisbinrahat/mcp-hub/internal/transport"
)

func Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "start":
		return runStart(ctx, args[1:])
	case "stop":
		return runStop(args[1:])
	case "status":
		return runStatus(args[1:])
	case "list":
		return runList(args[1:])
	case "sync":
		return runSync(args[1:])
	case "generate":
		fmt.Fprintln(os.Stderr, "Warning: 'mcp-hub generate' is deprecated. Use 'mcp-hub sync' instead.")
		return runGenerate(args[1:])
	case "add":
		return runAdd(args[1:])
	case "remove":
		return runRemove(args[1:])
	case "restart":
		if err := runStop(args[1:]); err != nil {
			return err
		}
		return runStart(ctx, args[1:])
	case "version":
		fmt.Printf("mcp-hub %s\n", hub.Version)
		return nil
	case "install":
		return runInstall(args[1:])
	case "uninstall":
		return runUninstall(args[1:])
	case "logs":
		return runLogs(args[1:])
	case "import":
		return runImport(args[1:])
	case "ide":
		return runIDE(args[1:])
	case "client":
		return runClient(args[1:])
	case "launch":
		return runLaunch(args[1:])
	default:
		return usageError()
	}
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func runStart(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	hotReload := fs.Bool("hot-reload", true, "watch config file and reload on change")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	logger, closeLog, err := newLogger(cfg.Hub.LogFile)
	if err != nil {
		return err
	}
	defer closeLog()

	if err := os.MkdirAll(cfg.BaseDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.PIDFile(), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
		return err
	}
	defer os.Remove(cfg.PIDFile())

	h := hub.New(cfg, logger)
	defer h.Close()

	if err := h.StartEager(ctx); err != nil {
		return err
	}

	mux := http.NewServeMux()
	admin.Mount(mux, h, cfg.Path, logger)
	mux.Handle("/ui/", admin.UIHandler())
	th := transport.NewHandler(h, logger)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.Method == http.MethodGet {
			http.Redirect(w, r, "/ui/", http.StatusFound)
			return
		}
		th.ServeHTTP(w, r)
	})
	server := &http.Server{
		Addr:    cfg.Hub.ListenAddress(),
		Handler: mux,
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *hotReload {
		go watchConfigReload(runCtx, h, logger, cfg.Path)
	}
	go func() {
		<-runCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("mcp-hub %s listening on %s", hub.Version, cfg.Hub.ListenAddress())
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func watchConfigReload(ctx context.Context, h *hub.Hub, logger *log.Logger, configPath string) {
	lastMod := time.Time{}
	if info, err := os.Stat(configPath); err == nil {
		lastMod = info.ModTime()
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(configPath)
			if err != nil {
				continue
			}
			if !info.ModTime().After(lastMod) {
				continue
			}
			nextCfg, err := config.Load(configPath)
			if err != nil {
				logger.Printf("hot-reload skipped: failed to load config: %v", err)
				lastMod = info.ModTime()
				continue
			}
			if err := h.Reload(context.Background(), nextCfg); err != nil {
				logger.Printf("hot-reload failed: %v", err)
			} else {
				logger.Printf("hot-reload applied from %s", configPath)
			}
			lastMod = info.ModTime()
		}
	}
}

func runStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(cfg.PIDFile())
	if err != nil {
		return err
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s/status", cfg.Hub.ListenAddress())
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if cfg.Hub.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Hub.APIKey)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("mcp-hub is not reachable on %s\n", cfg.Hub.ListenAddress())
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status request failed: %s", strings.TrimSpace(string(body)))
	}

	var snapshot hub.StatusSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return err
	}

	fmt.Printf("mcp-hub %s — %s\n\n", snapshot.Version, snapshot.Address)
	writer := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SERVER\tMODE\tSTATUS\tPID\tCLIENTS\tRESTARTS\tPING_MS\tLAST_PING\tLAST ERROR")
	for _, server := range snapshot.Servers {
		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
			server.Name,
			server.Mode,
			server.Status,
			server.PID,
			server.Clients,
			server.Restarts,
			server.LastPingMs,
			server.LastPingAt,
			server.LastError,
		)
	}
	return writer.Flush()
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	writer := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SERVER\tMODE\tTARGET\tLAZY")
	for _, name := range names {
		server := cfg.Servers[name]
		target := server.Command
		if server.URL != "" {
			target = server.URL
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%t\n", name, server.Mode, target, server.IsLazy())
	}
	return writer.Flush()
}

func runGenerate(args []string) error {
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	all := fs.Bool("all", false, "generate configs for all detected IDEs")
	dryRun := fs.Bool("dry-run", false, "print actions without writing files")
	backup := fs.Bool("backup", false, "create backup before writing")
	merge := fs.Bool("merge", false, "preserve non-hub servers in target config")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	opts := generate.Options{
		DryRun: *dryRun,
		Backup: *backup,
		Merge:  *merge,
	}

	if *all {
		results, err := generate.GenerateAllDetected(cfg, opts)
		if err != nil {
			return err
		}
		return printGenerateResults(results, *dryRun)
	}

	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub generate <ide>|--all [--dry-run] [--backup] [--merge] [--config path]")
	}
	target := fs.Arg(0)
	result, err := generate.Generate(cfg, target, opts)
	if err != nil {
		return err
	}
	return printGenerateResults([]generate.Result{result}, *dryRun)
}

func printGenerateResults(results []generate.Result, dryRun bool) error {
	action := "Generated"
	if dryRun {
		action = "Would generate"
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintf(w, "%s IDE configs:\n", action)
	fmt.Fprintln(w, "IDE\tSTATUS\tPATH\tSERVERS\tPRESERVED\tBACKUP")
	for _, res := range results {
		status := "written"
		if !res.Detected {
			status = "skipped(not detected)"
		} else if dryRun {
			status = "dry-run"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n", res.IDE, status, res.Path, res.ServerCount, res.Preserved, res.BackupPath)
	}
	return w.Flush()
}

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	command := fs.String("command", "", "server command for shared/isolated modes")
	url := fs.String("url", "", "remote URL for passthrough mode")
	mode := fs.String("mode", "", "server mode: shared|isolated|passthrough")
	lazy := fs.Bool("lazy", true, "lazy start server")
	var argValues stringSliceFlag
	var envValues stringSliceFlag
	var headerValues stringSliceFlag
	fs.Var(&argValues, "arg", "repeatable server arg value")
	fs.Var(&envValues, "env", "repeatable env KEY=VALUE")
	fs.Var(&headerValues, "header", "repeatable header KEY=VALUE")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub add <name> [--command cmd --arg x --mode shared|isolated] [--url http://... --mode passthrough]")
	}
	name := fs.Arg(0)

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	m := config.Mode(*mode)
	if m == "" {
		if *url != "" {
			m = config.ModePassthrough
		} else {
			m = config.ModeShared
		}
	}

	server := config.ServerConfig{
		Command: *command,
		Args:    append([]string(nil), argValues...),
		URL:     *url,
		Mode:    m,
		Lazy:    lazy,
		Env:     map[string]string{},
		Headers: map[string]string{},
	}
	for _, item := range envValues {
		k, v, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return fmt.Errorf("invalid --env value %q, expected KEY=VALUE", item)
		}
		server.Env[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	for _, item := range headerValues {
		k, v, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return fmt.Errorf("invalid --header value %q, expected KEY=VALUE", item)
		}
		server.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	switch server.Mode {
	case config.ModeShared, config.ModeIsolated:
		if server.Command == "" {
			return fmt.Errorf("--command is required for %s mode", server.Mode)
		}
		server.URL = ""
	case config.ModePassthrough:
		if server.URL == "" {
			return fmt.Errorf("--url is required for passthrough mode")
		}
		server.Command = ""
		server.Args = nil
		server.Env = nil
	default:
		return fmt.Errorf("unsupported mode %q", server.Mode)
	}

	cfg.Servers[name] = server
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Added server %q (%s)\n", name, server.Mode)
	return nil
}

func runRemove(args []string) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub remove <name>")
	}
	name := fs.Arg(0)

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}
	if _, ok := cfg.Servers[name]; !ok {
		return fmt.Errorf("server %q is not configured", name)
	}
	delete(cfg.Servers, name)
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Removed server %q\n", name)
	return nil
}

func newLogger(logPath string) (*log.Logger, func() error, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	logger := log.New(io.MultiWriter(os.Stderr, file), "", log.LstdFlags)
	return logger, file.Close, nil
}

func usageError() error {
	return fmt.Errorf("usage: mcp-hub <start|stop|status|restart|list|add|remove|generate|import|ide|install|uninstall|logs|version> [--config path]")
}
