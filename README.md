# mcp-hub

A centralized daemon that runs MCP servers once and shares them across all IDEs and AI apps.

## Features

- **Shared Mode**: Run each MCP server once, share across all clients
- **Isolated Mode**: One instance per client for stateful servers
- **Passthrough Mode**: Proxy to remote MCP servers
- **Aggregate Endpoint**: Single `/mcp` endpoint with namespaced tools
- **IDE Config Generator**: Auto-generate configs for 12+ IDEs
- **Cross-Platform**: macOS, Linux, and Windows support

## Quick Start

```bash
# Install from source
go install github.com/harisbinrahat/mcp-hub@latest

# Start the daemon
mcp-hub start

# Add a server
mcp-hub add context7 --command npx -- -y @upstash/context7-mcp@latest

# Generate client configs
mcp-hub sync --all

# Inspect runtime health
mcp-hub status
```

## Core Commands

```bash
# Daemon lifecycle
mcp-hub start
mcp-hub stop
mcp-hub restart

# Runtime and logs
mcp-hub status
mcp-hub logs --follow

# Server management
mcp-hub add <name> --command <cmd> -- <args...>
mcp-hub remove <name>
mcp-hub list

# Client config + launch
mcp-hub sync --all
mcp-hub launch <client-id>

# Service install
# macOS: launchd
# Linux: systemd --user
# Windows: Startup folder entry
mcp-hub install
mcp-hub uninstall
```

## Status Output

`mcp-hub status` includes:

- Aggregate endpoint health (`aggregate sessions`, `initialized`, `initializing`, `init_errors`)
- Per-server observability (`restarts`, `uptime_sec`, `last_restart_at`, `last_ping_ms`, `last_error`)

## Development

```bash
# Clone
git clone https://github.com/harisbinrahat/mcp-hub
cd mcp-hub

# Install dependencies
make install

# Build
make build

# Test
make test

# Run linter
make lint

# Full check before commit
make check
```

See [DEVELOPMENT.md](./DEVELOPMENT.md) for more details.

## Binaries

| Binary | Purpose |
|--------|---------|
| `mcp-hub` | Main CLI daemon |
| `mcp-hub-bridge` | Stdio-to-HTTP bridge for legacy clients |
| `mcp-hub-fixture` | Test MCP server |

## Release Artifacts

Cross-platform archives are produced via GoReleaser for:

- `mcp-hub` (`linux`, `darwin`, `windows`)
- `mcp-hub-bridge` (`linux`, `darwin`, `windows`)

Windows archives are `.zip`; non-Windows archives are `.tar.gz`.

## Documentation

- [SPEC.md](./SPEC.md) - Design specification
- [DEVELOPMENT.md](./DEVELOPMENT.md) - Development guide
- [AGENTS.md](./AGENTS.md) - Agent directives
