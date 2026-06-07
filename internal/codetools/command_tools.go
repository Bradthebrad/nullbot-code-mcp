package codetools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tinychain/mcp"
)

func (c *CodeTools) runCommandTool() mcp.Tool {
	return mcp.Tool{
		Name:        "run_command",
		Description: "Start a command asynchronously inside the workspace and return a command_id. Prefer command+args. Shell mode is available for user-approved shell snippets.",
		InputSchema: schema(map[string]any{
			"command":    stringProp("Executable name, or a shell command when shell=true."),
			"args":       arrayStringProp("Arguments passed to command when shell=false."),
			"cwd":        stringProp("Workspace-relative working directory. Defaults to workspace root."),
			"shell":      boolProp("Run through cmd.exe /C on Windows or sh -c elsewhere. Defaults to false."),
			"reason":     stringProp("Short explanation of why this command is being run."),
			"max_output": numberProp("Reserved for clients; server currently uses its configured max command output."),
		}, "command"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			cwd, err := c.resolve(textArg(args, "cwd"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			info, err := os.Stat(cwd)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			if !info.IsDir() {
				return mcp.ToolResult{}, fmt.Errorf("cwd is not a directory: %s", c.rel(cwd))
			}
			p, err := c.runner.start(textArg(args, "command"), stringSliceArg(args, "args"), cwd, boolArg(args, "shell"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(map[string]any{
				"command_id": p.id,
				"command":    p.command,
				"args":       p.args,
				"cwd":        c.rel(p.cwd),
				"started":    p.started.Format(time.RFC3339),
				"reason":     textArg(args, "reason"),
			})), nil
		},
	}
}

func (c *CodeTools) commandStatusTool() mcp.Tool {
	return mcp.Tool{
		Name:        "command_status",
		Description: "Check whether a previously started command is still running and inspect exit status when complete.",
		InputSchema: schema(map[string]any{
			"command_id": stringProp("Command id returned by run_command."),
		}, "command_id"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			p, ok := c.runner.get(textArg(args, "command_id"))
			if !ok {
				return mcp.ToolResult{}, fmt.Errorf("unknown command_id")
			}
			p.outputMu.Lock()
			defer p.outputMu.Unlock()
			status := "running"
			if p.finished != nil {
				status = "finished"
			}
			result := map[string]any{
				"command_id": p.id,
				"command":    p.command,
				"args":       p.args,
				"cwd":        filepath.ToSlash(c.rel(p.cwd)),
				"started":    p.started.Format(time.RFC3339),
				"status":     status,
			}
			if p.finished != nil {
				result["finished"] = p.finished.Format(time.RFC3339)
			}
			if p.exitCode != nil {
				result["exit_code"] = *p.exitCode
			}
			if p.err != "" {
				result["error"] = p.err
			}
			return mcp.Text(pretty(result)), nil
		},
	}
}

func (c *CodeTools) commandOutputTool() mcp.Tool {
	return mcp.Tool{
		Name:        "command_output",
		Description: "Return retained stdout/stderr for a command. Output is capped and may contain only the newest bytes if the command was noisy.",
		InputSchema: schema(map[string]any{
			"command_id": stringProp("Command id returned by run_command."),
			"tail_bytes": numberProp("Optional number of newest output bytes to return."),
		}, "command_id"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			p, ok := c.runner.get(textArg(args, "command_id"))
			if !ok {
				return mcp.ToolResult{}, fmt.Errorf("unknown command_id")
			}
			p.outputMu.Lock()
			defer p.outputMu.Unlock()
			output, truncated := p.output.snapshot()
			tailBytes := intArg(args, "tail_bytes", 0)
			if tailBytes > 0 && len(output) > tailBytes {
				output = output[len(output)-tailBytes:]
				truncated = true
			}
			status := "running"
			if p.finished != nil {
				status = "finished"
			}
			return mcp.Text(pretty(map[string]any{
				"command_id": p.id,
				"status":     status,
				"truncated":  truncated,
				"output":     output,
			})), nil
		},
	}
}

func (c *CodeTools) commandKillTool() mcp.Tool {
	return mcp.Tool{
		Name:        "command_kill",
		Description: "Request cancellation of a running command by command_id.",
		InputSchema: schema(map[string]any{
			"command_id": stringProp("Command id returned by run_command."),
		}, "command_id"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			p, ok := c.runner.get(textArg(args, "command_id"))
			if !ok {
				return mcp.ToolResult{}, fmt.Errorf("unknown command_id")
			}
			p.outputMu.Lock()
			done := p.finished != nil
			p.outputMu.Unlock()
			if !done {
				p.cancel()
			}
			return mcp.Text(pretty(map[string]any{
				"command_id":        p.id,
				"cancellation_sent": !done,
				"already_finished":  done,
			})), nil
		},
	}
}
