package codetools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tinychain/mcp"
)

func (c *CodeTools) workspaceInfoTool() mcp.Tool {
	return mcp.Tool{
		Name:        "workspace_info",
		Description: "Describe the active coding workspace root, path restrictions, and server limits. Use this before file operations when the workspace is unknown.",
		InputSchema: schema(map[string]any{}),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			return mcp.Text(pretty(map[string]any{
				"workspace":         c.root,
				"path_policy":       "all file paths must be relative to workspace and cannot escape it",
				"max_read_bytes":    c.maxReadBytes,
				"max_command_bytes": c.maxCommandBytes,
			})), nil
		},
	}
}

func (c *CodeTools) listDirTool() mcp.Tool {
	return mcp.Tool{
		Name:        "list_dir",
		Description: "List files and directories under a workspace-relative directory. Returns names, type, size, and modification time.",
		InputSchema: schema(map[string]any{
			"path":      stringProp("Workspace-relative directory path. Defaults to the workspace root."),
			"max_items": numberProp("Maximum entries to return. Defaults to 200."),
		}),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			dir, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			info, err := os.Stat(dir)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			if !info.IsDir() {
				return mcp.ToolResult{}, fmt.Errorf("%s is not a directory", c.rel(dir))
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			limit := intArg(args, "max_items", 200)
			if limit <= 0 || limit > 1000 {
				limit = 200
			}
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].IsDir() != entries[j].IsDir() {
					return entries[i].IsDir()
				}
				return entries[i].Name() < entries[j].Name()
			})
			var out []map[string]any
			for i, entry := range entries {
				if i >= limit {
					break
				}
				itemInfo, _ := entry.Info()
				item := map[string]any{
					"name": entry.Name(),
					"path": filepath.ToSlash(filepath.Join(c.rel(dir), entry.Name())),
					"type": "file",
				}
				if entry.IsDir() {
					item["type"] = "directory"
				}
				if itemInfo != nil {
					item["size"] = itemInfo.Size()
					item["mod_time"] = itemInfo.ModTime().Format("2006-01-02T15:04:05Z07:00")
				}
				out = append(out, item)
			}
			return mcp.Text(pretty(map[string]any{
				"path":      c.rel(dir),
				"returned":  len(out),
				"total":     len(entries),
				"truncated": len(entries) > len(out),
				"entries":   out,
			})), nil
		},
	}
}

func (c *CodeTools) fileInfoTool() mcp.Tool {
	return mcp.Tool{
		Name:        "file_info",
		Description: "Return metadata for a workspace-relative file or directory.",
		InputSchema: schema(map[string]any{
			"path": stringProp("Workspace-relative path."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			info, err := os.Stat(path)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			kind := "file"
			if info.IsDir() {
				kind = "directory"
			}
			return mcp.Text(pretty(map[string]any{
				"path":     c.rel(path),
				"type":     kind,
				"size":     info.Size(),
				"mode":     info.Mode().String(),
				"mod_time": info.ModTime().Format("2006-01-02T15:04:05Z07:00"),
			})), nil
		},
	}
}

func (c *CodeTools) readFileTool() mcp.Tool {
	return mcp.Tool{
		Name:        "read_file",
		Description: "Read a UTF-8-ish text file from the workspace. Supports optional 1-based line ranges and caps output to protect context.",
		InputSchema: schema(map[string]any{
			"path":       stringProp("Workspace-relative file path."),
			"start_line": numberProp("Optional 1-based first line to return."),
			"end_line":   numberProp("Optional 1-based final line to return, inclusive."),
			"max_bytes":  numberProp("Optional byte cap. Defaults to server max_read_bytes."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			info, err := os.Stat(path)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			if info.IsDir() {
				return mcp.ToolResult{}, fmt.Errorf("%s is a directory", c.rel(path))
			}
			maxBytes := int64(intArg(args, "max_bytes", int(c.maxReadBytes)))
			if maxBytes <= 0 || maxBytes > c.maxReadBytes {
				maxBytes = c.maxReadBytes
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			truncated := int64(len(data)) > maxBytes
			if truncated {
				data = data[:maxBytes]
			}
			text := string(data)
			start := intArg(args, "start_line", 0)
			end := intArg(args, "end_line", 0)
			if start > 0 || end > 0 {
				text = lineRange(text, start, end)
			}
			return mcp.Text(pretty(map[string]any{
				"path":      c.rel(path),
				"size":      info.Size(),
				"truncated": truncated,
				"content":   text,
			})), nil
		},
	}
}

func (c *CodeTools) writeFileTool() mcp.Tool {
	return mcp.Tool{
		Name:        "write_file",
		Description: "Create or replace a workspace-relative text file. Parent directories are created when needed. Use dry_run first for risky writes.",
		InputSchema: schema(map[string]any{
			"path":      stringProp("Workspace-relative file path."),
			"content":   stringProp("Full file content to write."),
			"overwrite": boolProp("Whether to replace an existing file. Defaults to false."),
			"dry_run":   boolProp("Preview the write without changing disk."),
		}, "path", "content"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			content := textArg(args, "content")
			overwrite := boolArg(args, "overwrite")
			dryRun := boolArg(args, "dry_run")
			if _, err := os.Stat(path); err == nil && !overwrite {
				return mcp.ToolResult{}, fmt.Errorf("%s exists; set overwrite=true to replace", c.rel(path))
			}
			if dryRun {
				return mcp.Text(pretty(map[string]any{
					"path":        c.rel(path),
					"would_write": len(content),
					"overwrite":   overwrite,
				})), nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return mcp.ToolResult{}, err
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(map[string]any{
				"path":    c.rel(path),
				"written": len(content),
			})), nil
		},
	}
}

func (c *CodeTools) editFileTool() mcp.Tool {
	return mcp.Tool{
		Name:        "edit_file",
		Description: "Perform exact string replacement in a workspace file. Fails unless the expected replacement count matches. Use dry_run to preview.",
		InputSchema: schema(map[string]any{
			"path":                  stringProp("Workspace-relative file path."),
			"old_text":              stringProp("Exact text to replace."),
			"new_text":              stringProp("Replacement text."),
			"expected_replacements": numberProp("Required number of replacements. Defaults to 1."),
			"dry_run":               boolProp("Preview without writing."),
		}, "path", "old_text", "new_text"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			oldText := textArg(args, "old_text")
			newText := textArg(args, "new_text")
			if oldText == "" {
				return mcp.ToolResult{}, fmt.Errorf("old_text is required")
			}
			expected := intArg(args, "expected_replacements", 1)
			count := strings.Count(string(data), oldText)
			if count != expected {
				return mcp.ToolResult{}, fmt.Errorf("expected %d replacements, found %d", expected, count)
			}
			updated := strings.ReplaceAll(string(data), oldText, newText)
			result := map[string]any{
				"path":         c.rel(path),
				"replacements": count,
				"old_bytes":    len(data),
				"new_bytes":    len(updated),
				"dry_run":      boolArg(args, "dry_run"),
			}
			if boolArg(args, "dry_run") {
				return mcp.Text(pretty(result)), nil
			}
			if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(result)), nil
		},
	}
}

func (c *CodeTools) makeDirTool() mcp.Tool {
	return mcp.Tool{
		Name:        "make_dir",
		Description: "Create a workspace-relative directory and parents if needed.",
		InputSchema: schema(map[string]any{
			"path": stringProp("Workspace-relative directory path."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			if err := os.MkdirAll(path, 0755); err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(map[string]any{"created": c.rel(path)})), nil
		},
	}
}

func (c *CodeTools) deletePathTool() mcp.Tool {
	return mcp.Tool{
		Name:        "delete_path",
		Description: "Delete a file or empty directory inside the workspace. Recursive deletion requires recursive=true and is intentionally explicit.",
		InputSchema: schema(map[string]any{
			"path":      stringProp("Workspace-relative path to delete."),
			"recursive": boolProp("Delete directories recursively. Defaults to false."),
			"dry_run":   boolProp("Preview without deleting."),
		}, "path"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			path, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			if path == c.root {
				return mcp.ToolResult{}, fmt.Errorf("refusing to delete workspace root")
			}
			info, err := os.Stat(path)
			if err != nil {
				return mcp.ToolResult{}, err
			}
			result := map[string]any{
				"path":      c.rel(path),
				"type":      "file",
				"recursive": boolArg(args, "recursive"),
				"dry_run":   boolArg(args, "dry_run"),
			}
			if info.IsDir() {
				result["type"] = "directory"
			}
			if boolArg(args, "dry_run") {
				return mcp.Text(pretty(result)), nil
			}
			if info.IsDir() && boolArg(args, "recursive") {
				err = os.RemoveAll(path)
			} else {
				err = os.Remove(path)
			}
			if err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(result)), nil
		},
	}
}

func lineRange(text string, start, end int) string {
	lines := strings.SplitAfter(text, "\n")
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) || start > end {
		return ""
	}
	return strings.Join(lines[start-1:end], "")
}

func skipDir(entry fs.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}
	switch entry.Name() {
	case ".git", ".hg", ".svn", "node_modules", ".gocache", ".gomodcache", "dist", "vendor":
		return true
	default:
		return false
	}
}
