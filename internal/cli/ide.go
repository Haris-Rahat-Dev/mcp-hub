package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/harisbinrahat/mcp-hub/internal/config"
	"github.com/harisbinrahat/mcp-hub/internal/generate"
)

// runClient is the canonical client management command.
func runClient(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mcp-hub client <add|remove|list>")
	}
	switch args[0] {
	case "add":
		return runClientAdd(args[1:])
	case "remove":
		return runClientRemove(args[1:])
	case "list":
		return runClientList(args[1:])
	default:
		return fmt.Errorf("usage: mcp-hub client <add|remove|list>")
	}
}

func runClientAdd(args []string) error {
	fs := flag.NewFlagSet("client add", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	path := fs.String("path", "", "path to client config file")
	key := fs.String("key", "mcpServers", "JSON key for MCP servers")
	urlKey := fs.String("url-key", "url", "JSON key for server URL")
	mode := fs.String("mode", "standalone", "file mode (standalone|merge)")
	kind := fs.String("kind", "gui", "client kind (gui|cli)")
	adapterMode := fs.String("adapter", "direct-http", "adapter mode (direct-http|bridge-stdio)")
	bundleID := fs.String("bundle-id", "", "macOS bundle identifier (for gui kind)")
	executable := fs.String("executable", "", "path to executable (for cli kind or launch)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub client add <id> --path <path> [flags]")
	}
	name := fs.Arg(0)
	if *path == "" {
		return fmt.Errorf("--path is required")
	}

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	if cfg.Clients == nil {
		cfg.Clients = make(map[string]config.ClientManifest)
	}
	cfg.Clients[name] = config.ClientManifest{
		DisplayName: name,
		Kind:        *kind,
		BundleID:    *bundleID,
		Executable:  *executable,
		ConfigPath:  *path,
		ConfigKey:   *key,
		URLKey:      *urlKey,
		FileMode:    *mode,
		Format:      "json",
		AdapterMode: *adapterMode,
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Added client %q\n", name)
	return nil
}

func runClientRemove(args []string) error {
	fs := flag.NewFlagSet("client remove", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub client remove <id>")
	}
	name := fs.Arg(0)

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	if _, ok := cfg.Clients[name]; !ok {
		return fmt.Errorf("client %q is not defined in config", name)
	}
	delete(cfg.Clients, name)

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Removed client %q\n", name)
	return nil
}

func runClientList(args []string) error {
	fs := flag.NewFlagSet("client list", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	clients := generate.ResolveClients(cfg)
	names := make([]string, 0, len(clients))
	for name := range clients {
		names = append(names, name)
	}
	sort.Strings(names)

	writer := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "CLIENT\tDISPLAY NAME\tKIND\tADAPTER\tCONFIG PATH\tSOURCE")
	for _, name := range names {
		m := clients[name]
		source := "built-in"
		if _, ok := cfg.Clients[name]; ok {
			source = "user-defined"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", name, m.DisplayName, m.Kind, m.AdapterMode, m.ConfigPath, source)
	}
	return writer.Flush()
}

// runIDE is a deprecated alias for runClient. Kept for backward compatibility.
func runIDE(args []string) error {
	fmt.Fprintln(os.Stderr, "Warning: 'mcp-hub ide' is deprecated. Use 'mcp-hub client' instead.")
	if len(args) == 0 {
		return fmt.Errorf("usage: mcp-hub ide <add|remove|list>")
	}
	switch args[0] {
	case "add":
		return runIDEAdd(args[1:])
	case "remove":
		return runIDERemove(args[1:])
	case "list":
		return runIDEList(args[1:])
	default:
		return fmt.Errorf("usage: mcp-hub ide <add|remove|list>")
	}
}

func runIDEAdd(args []string) error {
	fs := flag.NewFlagSet("ide add", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	path := fs.String("path", "", "path to IDE config file")
	key := fs.String("key", "mcpServers", "JSON key for MCP servers")
	urlKey := fs.String("url-key", "url", "JSON key for server URL")
	mode := fs.String("mode", "standalone", "file mode (standalone|merge)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub ide add <name> --path <path> [--key mcpServers] [--url-key url] [--mode standalone|merge]")
	}
	name := fs.Arg(0)
	if *path == "" {
		return fmt.Errorf("--path is required")
	}

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	if cfg.IDEs == nil {
		cfg.IDEs = make(map[string]config.IDEConfig)
	}
	cfg.IDEs[name] = config.IDEConfig{
		ConfigPath: *path,
		ConfigKey:  *key,
		URLKey:     *urlKey,
		FileMode:   *mode,
	}

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Added IDE %q\n", name)
	return nil
}

func runIDERemove(args []string) error {
	fs := flag.NewFlagSet("ide remove", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: mcp-hub ide remove <name>")
	}
	name := fs.Arg(0)

	cfg, err := config.LoadForMutation(*configPath)
	if err != nil {
		return err
	}

	if _, ok := cfg.IDEs[name]; !ok {
		return fmt.Errorf("IDE %q is not defined in config", name)
	}
	delete(cfg.IDEs, name)

	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("Removed IDE %q\n", name)
	return nil
}

func runIDEList(args []string) error {
	fs := flag.NewFlagSet("ide list", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	defs := generate.ResolveIDEs(cfg)
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)

	writer := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "IDE\tCONFIG PATH\tKEY\tURL KEY\tMODE\tSOURCE")
	for _, name := range names {
		def := defs[name]
		source := "built-in"
		if _, ok := cfg.IDEs[name]; ok {
			source = "user-defined"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", def.Name, def.ConfigPath, def.ConfigKey, def.URLKey, def.FileMode, source)
	}
	return writer.Flush()
}
