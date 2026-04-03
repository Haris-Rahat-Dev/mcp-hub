# mcp-hub

MCP hub in Go: shared, isolated, and passthrough server modes with an aggregate `/mcp` endpoint.

See `go.mod` for the module path and `cmd/` for entrypoints (`mcp-hub`, `mcp-hub-bridge`, `mcp-hub-fixture`).

## Build

```bash
go build -o mcp-hub ./cmd/mcp-hub
```

Replace the output name as needed for bridge or fixture binaries.
