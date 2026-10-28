package codetools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Workspace       string
	MaxReadBytes    int64
	MaxCommandBytes int
}

type CodeTools struct {
	root            string
	maxReadBytes    int64
	maxCommandBytes int
	runner          *runner
}

func New(config Config) (*CodeTools, error) {
	workspace := config.Workspace
	if workspace == "" {
		workspace = "."
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory: %s", root)
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	maxReadBytes := config.MaxReadBytes
	if maxReadBytes <= 0 {
		maxReadBytes = 512 * 1024
	}
	maxCommandBytes := config.MaxCommandBytes
	if maxCommandBytes <= 0 {
		maxCommandBytes = 128 * 1024
	}
	return &CodeTools{
		root:            root,
		maxReadBytes:    maxReadBytes,
		maxCommandBytes: maxCommandBytes,
		runner:          newRunner(root, maxCommandBytes),
	}, nil
}

// resolve turns a tool's path argument into a path inside the workspace,
// relative to it or absolute. Links that lead outside are refused, whether
// they are folders or files.
func (c *CodeTools) resolve(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		rel = "."
	}
	return confine(c.root, rel)
}

func (c *CodeTools) rel(path string) string {
	rel, err := filepath.Rel(c.root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
