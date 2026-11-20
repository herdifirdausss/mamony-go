# Experimental gopls MCP — local, opt-in only

Go's official [gopls MCP documentation](https://go.dev/gopls/features/mcp) describes an experimental language-server-backed MCP server with `gopls mcp` (detached stdio mode) and `gopls serve -mcp.listen=localhost:8092` (attached mode).

For an already installed and compatible gopls:

```sh
gopls version
gopls mcp -instructions > /tmp/mamony-gopls-instructions.md
# Example only for an opted-in Claude Code client:
claude mcp add gopls -- gopls mcp
claude mcp list
```

The above **is documentation**, not an action this repository executed or installed. Consult your specific coding agent's current MCP configuration documentation before enabling it; don't commit home-directory client credentials or auto-install tool permissions.

## Use / verify

Only activate for symbol definition, references, type hierarchy and package-aware navigation where it gives better results than focused Go source reads. Check MCP initialization and tool listings in the actual client before claiming connected. Run in a sandbox because gopls tools can operate with local workspace privileges. Never auto-expose an MCP listener on a public interface.

Experimental features can change. This is **NOT CONFIGURED** in the CI or ChatGPT GitHub connection.
