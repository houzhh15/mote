# Mote MCP Configuration Guide

Mote has built-in MCP (Model Context Protocol) support. You can configure and manage MCP servers using the following tools.

## Available MCP Tools

### 1. Add an MCP Server

Use the `mcp_add` tool to register a new MCP server.

**CRITICAL RULES for mcp_add:**
1. When user provides a JSON config or mentions headers/Authorization, you MUST extract and include ALL parameters
2. The `headers` parameter is REQUIRED if the user provides any Authorization token
3. Copy the user's headers exactly as provided

**For HTTP-based servers with Authorization:**
```json
{
  "name": "my-server",
  "type": "http",
  "url": "http://127.0.0.1:8001/mcp",
  "headers": {"Authorization": "Bearer <token>"}
}
```

**Example:** If user says "设置MCP: local, http://127.0.0.1:8001/mcp, headers有Authorization: Bearer xyz"
You MUST call mcp_add with:
```json
{
  "name": "local",
  "type": "http", 
  "url": "http://127.0.0.1:8001/mcp",
  "headers": {"Authorization": "Bearer xyz"}
}
```

**For stdio-based servers:**
```json
{
  "name": "filesystem",
  "type": "stdio",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
}
```

### 2. List MCP Servers

Use the `mcp_list` tool to see all configured servers and their connection status.

### 3. Call MCP Tools

Use the `mcp_call` tool to invoke a tool from a connected MCP server:

```json
{
  "server": "filesystem",
  "tool": "read_file",
  "arguments": {"path": "/tmp/example.txt"}
}
```

### 4. Remove an MCP Server

Use the `mcp_remove` tool with the server name to remove a configured server:

```json
{
  "name": "my-server"
}
```

## Common MCP Servers

Here are some popular MCP servers you can configure:

| Server | Purpose | Command |
|--------|---------|---------|
| Filesystem | File operations | `npx -y @modelcontextprotocol/server-filesystem /path` |
| Brave Search | Web search | `npx -y @anthropic-ai/mcp-server-brave-search` |
| GitHub | GitHub API | `npx -y @modelcontextprotocol/server-github` |
| Postgres | Database | `npx -y @modelcontextprotocol/server-postgres` |

## Configuration File

MCP servers can also be configured in `~/.mote/config.yaml`:

```yaml
mcp:
  client:
    servers:
      - name: "filesystem"
        command: "npx"
        args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      - name: "brave-search"
        command: "npx"
        args: ["-y", "@anthropic-ai/mcp-server-brave-search"]
        env:
          BRAVE_API_KEY: "${BRAVE_API_KEY}"
```

## Best Practices

1. **Use descriptive names** for servers to easily identify them later
2. **Set environment variables** for API keys rather than hardcoding them
3. **Test connections** with `mcp_list` after adding a server
4. **Remove unused servers** to keep configuration clean
