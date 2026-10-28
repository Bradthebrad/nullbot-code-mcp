package codetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Bradthebrad/tinychain/mcp"
)

func TestToolsListAndFileWorkflow(t *testing.T) {
	dir := t.TempDir()
	codeTools, err := New(Config{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.NewServer("test")
	for _, tool := range codeTools.Tools() {
		server.AddTool(tool)
	}

	list := callTool(t, server, "tools/list", nil)
	data, _ := json.Marshal(list.Result)
	var tools mcp.ListToolsResult
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) < 10 {
		t.Fatalf("expected coding tools, got %d", len(tools.Tools))
	}

	callMCPTool(t, server, "write_file", map[string]any{
		"path":    "src/main.go",
		"content": "package main\n\nfunc main() {}\n",
	})
	callMCPTool(t, server, "edit_file", map[string]any{
		"path":     "src/main.go",
		"old_text": "func main() {}",
		"new_text": "func main() { println(\"hi\") }",
		"dry_run":  true,
	})
	callMCPTool(t, server, "edit_file", map[string]any{
		"path":     "src/main.go",
		"old_text": "func main() {}",
		"new_text": "func main() { println(\"hi\") }",
	})

	read := resultText(callMCPTool(t, server, "read_file", map[string]any{"path": "src/main.go"}))
	if !strings.Contains(read, "println") {
		t.Fatalf("read_file result = %s", read)
	}
	search := resultText(callMCPTool(t, server, "search_text", map[string]any{
		"query":        "println",
		"file_pattern": "*.go",
	}))
	if !strings.Contains(search, "src/main.go") {
		t.Fatalf("search_text result = %s", search)
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "main.go")); err != nil {
		t.Fatal(err)
	}
}

func TestCommandLifecycle(t *testing.T) {
	codeTools, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.NewServer("test")
	for _, tool := range codeTools.Tools() {
		server.AddTool(tool)
	}

	start := resultText(callMCPTool(t, server, "run_command", map[string]any{
		"command": "echo hello",
		"shell":   true,
	}))
	var started map[string]any
	if err := json.Unmarshal([]byte(start), &started); err != nil {
		t.Fatal(err)
	}
	id, _ := started["command_id"].(string)
	if id == "" {
		t.Fatalf("missing command id: %s", start)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		statusText := resultText(callMCPTool(t, server, "command_status", map[string]any{"command_id": id}))
		if strings.Contains(statusText, `"status": "finished"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("command did not finish: %s", statusText)
		}
		time.Sleep(20 * time.Millisecond)
	}

	output := resultText(callMCPTool(t, server, "command_output", map[string]any{"command_id": id}))
	if !strings.Contains(output, "hello") {
		t.Fatalf("command output = %s", output)
	}
}

func TestWindowsShellBuiltinsAreShellRouted(t *testing.T) {
	if shouldUseWindowsShell("dir", nil) != (runtime.GOOS == "windows") {
		t.Fatalf("dir shell routing mismatch for %s", runtime.GOOS)
	}
	if shouldUseWindowsShell("go", []string{"test"}) {
		t.Fatal("real executable with args should not be shell-routed")
	}
}

func callMCPTool(t *testing.T, server *mcp.Server, name string, args map[string]any) mcp.ToolResult {
	t.Helper()
	params, _ := json.Marshal(mcp.CallToolParams{Name: name, Arguments: args})
	resp := callTool(t, server, "tools/call", params)
	if resp.Error != nil {
		t.Fatalf("%s error: %s", name, resp.Error.Message)
	}
	data, _ := json.Marshal(resp.Result)
	var result mcp.ToolResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func callTool(t *testing.T, server *mcp.Server, method string, params json.RawMessage) mcp.Response {
	t.Helper()
	resp := server.Handle(context.Background(), mcp.Request{JSONRPC: mcp.JSONRPCVersion, ID: 1, Method: method, Params: params})
	if resp.Error != nil {
		t.Fatalf("%s error: %s", method, resp.Error.Message)
	}
	return resp
}

func resultText(result mcp.ToolResult) string {
	var out strings.Builder
	for _, content := range result.Content {
		out.WriteString(content.Text)
	}
	return out.String()
}

func TestPathsMayBeAbsoluteInsideTheWorkspaceButNotLinkedOut(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{Workspace: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.resolve(filepath.Join(c.root, "main.go")); err != nil || filepath.Base(got) != "main.go" {
		t.Fatalf("absolute path inside the workspace: %q, %v", got, err)
	}
	if got, err := c.resolve(""); err != nil || got != c.root {
		t.Fatalf("empty path should be the workspace: %q, %v", got, err)
	}
	if _, err := c.resolve(filepath.Join(outside, "secret.txt")); err == nil {
		t.Fatal("absolute path outside the workspace accepted")
	}
	// A linked file with an extension used to be followed out of the workspace.
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "notes.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := c.resolve("notes.txt"); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("linked file outside the workspace: %v", err)
	}
}
