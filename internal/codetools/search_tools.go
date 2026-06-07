package codetools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tinychain/mcp"
)

func (c *CodeTools) searchFilesTool() mcp.Tool {
	return mcp.Tool{
		Name:        "search_files",
		Description: "Find workspace files by glob-style name pattern. Useful for locating modules, configs, tests, docs, and generated files without reading file content.",
		InputSchema: schema(map[string]any{
			"pattern":   stringProp("Glob pattern matched against slash paths and base names, such as *.go, **/README.md, or cmd/*/main.go."),
			"path":      stringProp("Optional workspace-relative directory to search. Defaults to workspace root."),
			"max_items": numberProp("Maximum files to return. Defaults to 200."),
		}, "pattern"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			root, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			pattern := filepath.ToSlash(textArg(args, "pattern"))
			limit := intArg(args, "max_items", 200)
			if limit <= 0 || limit > 2000 {
				limit = 200
			}
			var matches []string
			err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if skipDir(entry) && path != root {
					return filepath.SkipDir
				}
				if entry.IsDir() {
					return nil
				}
				rel := c.rel(path)
				ok, _ := filepath.Match(pattern, filepath.ToSlash(entry.Name()))
				if !ok {
					ok, _ = filepath.Match(pattern, rel)
				}
				if !ok && strings.Contains(pattern, "**/") {
					ok, _ = filepath.Match(strings.ReplaceAll(pattern, "**/", ""), rel)
				}
				if ok {
					matches = append(matches, rel)
				}
				return nil
			})
			if err != nil {
				return mcp.ToolResult{}, err
			}
			sort.Strings(matches)
			truncated := len(matches) > limit
			if truncated {
				matches = matches[:limit]
			}
			return mcp.Text(pretty(map[string]any{
				"pattern":   pattern,
				"returned":  len(matches),
				"truncated": truncated,
				"files":     matches,
			})), nil
		},
	}
}

func (c *CodeTools) searchTextTool() mcp.Tool {
	return mcp.Tool{
		Name:        "search_text",
		Description: "Search text files in the workspace by literal text or regular expression. Returns file paths, line numbers, and matching line snippets.",
		InputSchema: schema(map[string]any{
			"query":        stringProp("Text or regular expression to search for."),
			"path":         stringProp("Optional workspace-relative directory or file to search. Defaults to workspace root."),
			"file_pattern": stringProp("Optional glob pattern for file names or slash paths, such as *.go or **/*.md."),
			"regex":        boolProp("Treat query as a regular expression. Defaults to false."),
			"ignore_case":  boolProp("Case-insensitive search."),
			"max_matches":  numberProp("Maximum matching lines to return. Defaults to 100."),
		}, "query"),
		Handler: func(ctx context.Context, args map[string]any) (mcp.ToolResult, error) {
			start, err := c.resolve(textArg(args, "path"))
			if err != nil {
				return mcp.ToolResult{}, err
			}
			query := textArg(args, "query")
			if query == "" {
				return mcp.ToolResult{}, fmt.Errorf("query is required")
			}
			limit := intArg(args, "max_matches", 100)
			if limit <= 0 || limit > 1000 {
				limit = 100
			}
			var matcher func(string) bool
			if boolArg(args, "regex") {
				pattern := query
				if boolArg(args, "ignore_case") {
					pattern = "(?i)" + pattern
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return mcp.ToolResult{}, err
				}
				matcher = re.MatchString
			} else if boolArg(args, "ignore_case") {
				needle := strings.ToLower(query)
				matcher = func(line string) bool { return strings.Contains(strings.ToLower(line), needle) }
			} else {
				matcher = func(line string) bool { return strings.Contains(line, query) }
			}

			filePattern := filepath.ToSlash(textArg(args, "file_pattern"))
			var matches []map[string]any
			err = filepath.WalkDir(start, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if len(matches) >= limit {
					return filepath.SkipAll
				}
				if skipDir(entry) && path != start {
					return filepath.SkipDir
				}
				if entry.IsDir() {
					return nil
				}
				rel := c.rel(path)
				if filePattern != "" && !matchesPattern(filePattern, rel, entry.Name()) {
					return nil
				}
				fileMatches, err := c.searchFile(path, matcher, limit-len(matches))
				if err != nil {
					return nil
				}
				for _, match := range fileMatches {
					match["path"] = rel
					matches = append(matches, match)
				}
				return nil
			})
			if err != nil {
				return mcp.ToolResult{}, err
			}
			return mcp.Text(pretty(map[string]any{
				"query":      query,
				"returned":   len(matches),
				"limit":      limit,
				"truncated":  len(matches) >= limit,
				"matches":    matches,
				"regex":      boolArg(args, "regex"),
				"ignoreCase": boolArg(args, "ignore_case"),
			})), nil
		},
	}
}

func (c *CodeTools) searchFile(path string, matcher func(string) bool, limit int) ([]map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var matches []map[string]any
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if matcher(line) {
			matches = append(matches, map[string]any{
				"line": lineNo,
				"text": truncate(line, 500),
			})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, scanner.Err()
}

func matchesPattern(pattern, rel, name string) bool {
	if ok, _ := filepath.Match(pattern, filepath.ToSlash(name)); ok {
		return true
	}
	if ok, _ := filepath.Match(pattern, rel); ok {
		return true
	}
	if strings.Contains(pattern, "**/") {
		ok, _ := filepath.Match(strings.ReplaceAll(pattern, "**/", ""), rel)
		return ok
	}
	return false
}
