package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeShared      Mode = "shared"
	ModeIsolated    Mode = "isolated"
	ModePassthrough Mode = "passthrough"
)

// ClientManifest fully describes a client application that mcp-hub manages.
type ClientManifest struct {
	DisplayName   string            `yaml:"display_name"`
	Kind          string            `yaml:"kind"` // "gui" | "cli"
	BundleID      string            `yaml:"bundle_id,omitempty"`
	Executable    string            `yaml:"executable,omitempty"`
	ConfigPath    string            `yaml:"config_path"`
	ConfigKey     string            `yaml:"config_key"`
	URLKey        string            `yaml:"url_key"`
	FileMode      string            `yaml:"file_mode"` // "standalone" | "merge"
	Format        string            `yaml:"format"`    // "json" | "yaml" | "toml"
	ExtraFields   map[string]string `yaml:"extra_fields,omitempty"`
	AdapterMode   string            `yaml:"adapter_mode"` // "direct-http" | "bridge-stdio"
	BridgeCommand string            `yaml:"bridge_command,omitempty"`
	BridgeArgs    []string          `yaml:"bridge_args,omitempty"`
}

type Config struct {
	Path    string                    `yaml:"-"`
	Hub     HubConfig                 `yaml:"hub"`
	Servers map[string]ServerConfig   `yaml:"servers"`
	Clients map[string]ClientManifest `yaml:"clients,omitempty"`
	IDEs    map[string]IDEConfig      `yaml:"ides,omitempty"`
}

type HubConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	LogLevel string `yaml:"log_level"`
	LogFile  string `yaml:"log_file"`
	APIKey   string `yaml:"api_key,omitempty"`
}

type ServerConfig struct {
	Command      string                    `yaml:"command,omitempty"`
	Args         []string                  `yaml:"args,omitempty"`
	Env          map[string]string         `yaml:"env,omitempty"`
	URL          string                    `yaml:"url,omitempty"`
	Headers      map[string]string         `yaml:"headers,omitempty"`
	Mode         Mode                      `yaml:"mode,omitempty"`
	Lazy         *bool                     `yaml:"lazy,omitempty"`
	MaxInstances int                       `yaml:"max_instances,omitempty"`
	Passthrough  map[string]map[string]any `yaml:"passthrough,omitempty"`
}

type IDEConfig struct {
	ConfigPath  string            `yaml:"config_path,omitempty"`
	ConfigKey   string            `yaml:"config_key,omitempty"`
	URLKey      string            `yaml:"url_key,omitempty"`
	FileMode    string            `yaml:"file_mode,omitempty"`
	ExtraFields map[string]string `yaml:"extra_fields,omitempty"`
	Format      string            `yaml:"format,omitempty"`
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func ResolveConfigPath(path string) (string, error) {
	if path != "" {
		return expandPath(path)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	candidates := platformConfigCandidates(home)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return candidates[0], nil
}

func platformConfigCandidates(home string) []string {
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata != "" {
			return []string{
				filepath.Join(appdata, "mcp-hub", "config.yaml"),
				filepath.Join(home, ".config", "mcp-hub", "config.yaml"),
				filepath.Join(home, ".mcp-hub", "config.yaml"),
			}
		}
	}
	return []string{
		filepath.Join(home, ".config", "mcp-hub", "config.yaml"),
		filepath.Join(home, ".mcp-hub", "config.yaml"),
	}
}

func defaultConfigBaseDir(home string) string {
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		if appdata != "" {
			return filepath.Join(appdata, "mcp-hub")
		}
	}
	return filepath.Join(home, ".config", "mcp-hub")
}

func Load(path string) (Config, error) {
	resolvedPath, err := ResolveConfigPath(path)
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	cfg.Path = resolvedPath
	cfg.applyDefaults()

	envValues, err := loadEnvironment(filepath.Dir(resolvedPath))
	if err != nil {
		return Config{}, err
	}

	if err := cfg.expand(envValues); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func LoadForMutation(path string) (Config, error) {
	resolvedPath, err := ResolveConfigPath(path)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Path = resolvedPath
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	if cfg.Path == "" {
		resolved, err := ResolveConfigPath("")
		if err != nil {
			return err
		}
		cfg.Path = resolved
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.Path, data, 0o644)
}

// PrepareAndValidate applies defaults and validation after YAML unmarshal (e.g. admin API).
func (c *Config) PrepareAndValidate() error {
	c.applyDefaults()
	return c.validate()
}

func (c Config) BaseDir() string {
	if c.Path == "" {
		return ""
	}
	return filepath.Dir(c.Path)
}

func (c Config) PIDFile() string {
	return filepath.Join(c.BaseDir(), "hub.pid")
}

func (h HubConfig) ListenAddress() string {
	return fmt.Sprintf("%s:%d", h.Host, h.Port)
}

func (s ServerConfig) IsLazy() bool {
	if s.Lazy == nil {
		return s.Mode != ModePassthrough
	}
	return *s.Lazy
}

func (c *Config) applyDefaults() {
	if c.Hub.Host == "" {
		c.Hub.Host = "127.0.0.1"
	}
	if c.Hub.Port == 0 {
		c.Hub.Port = 4117
	}
	if c.Hub.LogLevel == "" {
		c.Hub.LogLevel = "info"
	}
	if c.Hub.LogFile == "" {
		home, _ := os.UserHomeDir()
		baseDir := defaultConfigBaseDir(home)
		c.Hub.LogFile = filepath.Join(baseDir, "hub.log")
	}
	if c.Servers == nil {
		c.Servers = map[string]ServerConfig{}
	}
	if c.IDEs == nil {
		c.IDEs = map[string]IDEConfig{}
	}
	if c.Clients == nil {
		c.Clients = map[string]ClientManifest{}
	}
	for name, server := range c.Servers {
		if server.Mode == "" {
			server.Mode = ModeShared
		}
		c.Servers[name] = server
	}
	for name, ide := range c.IDEs {
		if _, exists := c.Clients[name]; exists {
			continue
		}
		displayName := name
		m := ClientManifest{
			DisplayName: displayName,
			Kind:        "gui",
			ConfigPath:  ide.ConfigPath,
			ConfigKey:   ide.ConfigKey,
			URLKey:      ide.URLKey,
			FileMode:    ide.FileMode,
			Format:      ide.Format,
			ExtraFields: ide.ExtraFields,
			AdapterMode: "direct-http",
		}
		if m.ConfigKey == "" {
			m.ConfigKey = "mcpServers"
		}
		if m.URLKey == "" {
			m.URLKey = "url"
		}
		if m.FileMode == "" {
			m.FileMode = "standalone"
		}
		if m.Format == "" {
			m.Format = "json"
		}
		c.Clients[name] = m
	}
}

func (c *Config) expand(envValues map[string]string) error {
	var err error
	c.Hub.LogFile, err = expandString(c.Hub.LogFile, envValues)
	if err != nil {
		return err
	}
	c.Hub.APIKey, err = expandString(c.Hub.APIKey, envValues)
	if err != nil {
		return err
	}
	c.Hub.LogFile, err = expandPath(c.Hub.LogFile)
	if err != nil {
		return err
	}

	serverNames := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, name := range serverNames {
		server := c.Servers[name]
		server.Command, err = expandString(server.Command, envValues)
		if err != nil {
			return fmt.Errorf("servers.%s.command: %w", name, err)
		}
		server.URL, err = expandString(server.URL, envValues)
		if err != nil {
			return fmt.Errorf("servers.%s.url: %w", name, err)
		}
		for i, arg := range server.Args {
			server.Args[i], err = expandString(arg, envValues)
			if err != nil {
				return fmt.Errorf("servers.%s.args[%d]: %w", name, i, err)
			}
			server.Args[i], err = expandPath(server.Args[i])
			if err != nil {
				return fmt.Errorf("servers.%s.args[%d]: %w", name, i, err)
			}
		}
		for key, value := range server.Env {
			server.Env[key], err = expandString(value, envValues)
			if err != nil {
				return fmt.Errorf("servers.%s.env.%s: %w", name, key, err)
			}
		}
		for key, value := range server.Headers {
			server.Headers[key], err = expandString(value, envValues)
			if err != nil {
				return fmt.Errorf("servers.%s.headers.%s: %w", name, key, err)
			}
		}
		server.Command, err = expandPath(server.Command)
		if err != nil {
			return fmt.Errorf("servers.%s.command: %w", name, err)
		}
		c.Servers[name] = server
	}

	for name, ide := range c.IDEs {
		ide.ConfigPath, err = expandString(ide.ConfigPath, envValues)
		if err != nil {
			return fmt.Errorf("ides.%s.config_path: %w", name, err)
		}
		ide.ConfigPath, err = expandPath(ide.ConfigPath)
		if err != nil {
			return fmt.Errorf("ides.%s.config_path: %w", name, err)
		}
		c.IDEs[name] = ide
	}

	for name, client := range c.Clients {
		client.ConfigPath, err = expandString(client.ConfigPath, envValues)
		if err != nil {
			return fmt.Errorf("clients.%s.config_path: %w", name, err)
		}
		client.ConfigPath, err = expandPath(client.ConfigPath)
		if err != nil {
			return fmt.Errorf("clients.%s.config_path: %w", name, err)
		}
		client.Executable, err = expandString(client.Executable, envValues)
		if err != nil {
			return fmt.Errorf("clients.%s.executable: %w", name, err)
		}
		client.Executable, err = expandPath(client.Executable)
		if err != nil {
			return fmt.Errorf("clients.%s.executable: %w", name, err)
		}
		c.Clients[name] = client
	}

	return nil
}

func (c Config) validate() error {
	if len(c.Servers) == 0 {
		return errors.New("config must define at least one server")
	}
	for name, server := range c.Servers {
		switch server.Mode {
		case ModeShared, ModeIsolated:
			if server.Command == "" {
				return fmt.Errorf("servers.%s.command is required for %s mode", name, server.Mode)
			}
			if server.URL != "" {
				return fmt.Errorf("servers.%s.url is not valid for %s mode", name, server.Mode)
			}
		case ModePassthrough:
			if server.URL == "" {
				return fmt.Errorf("servers.%s.url is required for passthrough mode", name)
			}
		default:
			return fmt.Errorf("servers.%s.mode %q is not supported", name, server.Mode)
		}
	}
	return nil
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "mcp-hub", "config.yaml")
	}
	return filepath.Join(defaultConfigBaseDir(home), "config.yaml")
}

func loadEnvironment(baseDir string) (map[string]string, error) {
	values := map[string]string{}
	envPath := filepath.Join(baseDir, ".env")
	if file, err := os.Open(envPath); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				continue
			}
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}

	return values, nil
}

func expandString(value string, envValues map[string]string) (string, error) {
	if value == "" {
		return "", nil
	}
	var err error
	out := envPattern.ReplaceAllStringFunc(value, func(match string) string {
		if err != nil {
			return ""
		}
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		resolved, ok := envValues[parts[1]]
		if !ok {
			err = fmt.Errorf("missing environment variable %s", parts[1])
			return ""
		}
		return resolved
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}
