# nullbot-code-mcp

`nullbot-code-mcp` is a small Go MCP server that exposes coding tools over stdio by default, with optional localhost HTTP and SSE transports.

It is designed for NullBot, but it is not NullBot-specific. Any MCP-capable client that can launch a stdio binary or connect to a local MCP endpoint can use it.

## Tools

| Tool | Purpose |
| --- | --- |
| `workspace_info` | Describe the active workspace root, path policy, and server limits. |
| `list_dir` | List files/directories with metadata. |
| `file_info` | Inspect one file or directory. |
| `read_file` | Read bounded text content with optional line ranges. |
| `write_file` | Create or overwrite text files, with `dry_run` support. |
| `edit_file` | Exact string replacement with required replacement counts and `dry_run` support. |
| `make_dir` | Create a directory and parents. |
| `delete_path` | Delete a file or directory, with explicit recursive mode and `dry_run`. |
| `search_files` | Find files by glob-style patterns. |
| `search_text` | Search file content by literal text or regex. |
| `run_command` | Start a workspace-scoped command asynchronously. |
| `command_status` | Check command state and exit code. |
| `command_output` | Read retained stdout/stderr from a command. |
| `command_kill` | Cancel a running command. |

## Safety Model

All file paths are workspace-relative. Absolute paths are rejected, and file operations are checked so they cannot escape the configured workspace root.

Command execution is intentionally included because this is a coding MCP server. Clients should still ask users before running risky commands. The server starts commands asynchronously and caps retained output so noisy commands do not flood agent context.

## Build

```powershell
go build -trimpath -ldflags "-s -w" -o nullbot-code-mcp.exe ./cmd/nullbot-code-mcp
```

## Run

Default stdio mode:

```powershell
.\nullbot-code-mcp.exe --workspace C:\path\to\project
```

Streamable HTTP-style endpoint:

```powershell
.\nullbot-code-mcp.exe --transport streamable-http --addr 127.0.0.1:8765 --path /mcp --workspace C:\path\to\project
```

Legacy SSE-compatible endpoints:

```powershell
.\nullbot-code-mcp.exe --transport sse --addr 127.0.0.1:8765 --sse-path /sse --message-path /message --workspace C:\path\to\project
```

## Claude / Other MCP Clients

Use the compiled executable as a stdio MCP server. The exact config format varies by client, but the command should point to `nullbot-code-mcp.exe` and pass a `--workspace` argument.

Conceptually:

```json
{
  "mcpServers": {
    "code": {
      "command": "C:\\path\\to\\nullbot-code-mcp.exe",
      "args": ["--workspace", "C:\\path\\to\\project"]
    }
  }
}
```

## Transport Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--transport` | `stdio` | `stdio`, `streamable-http`, `http`, or `sse`. |
| `--addr` | `127.0.0.1:8765` | Listen address for HTTP/SSE transports. |
| `--path` | `/mcp` | Streamable HTTP JSON-RPC endpoint. |
| `--sse-path` | `/sse` | Legacy SSE endpoint. |
| `--message-path` | `/message` | Legacy SSE message endpoint. |
| `--workspace` | `.` | Workspace root for coding tools. |
| `--max-read-bytes` | `524288` | Maximum bytes returned by `read_file`. |
| `--max-command-bytes` | `131072` | Retained command output bytes per command. |

## Development

```powershell
go test ./...
```
