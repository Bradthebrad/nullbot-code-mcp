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

func (c *CodeTools) resolve(rel string) (string, error) {
	if rel == "" {
		rel = "."
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", rel)
	}
	clean := filepath.Clean(rel)
	full := filepath.Join(c.root, clean)
	parent := full
	if filepath.Ext(full) != "" {
		parent = filepath.Dir(full)
	}
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		if err := c.ensureInside(resolved); err != nil {
			return "", err
		}
	}
	if err := c.ensureInside(full); err != nil {
		return "", err
	}
	return full, nil
}

func (c *CodeTools) ensureInside(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if abs == c.root {
		return nil
	}
	prefix := c.root + string(os.PathSeparator)
	if !strings.HasPrefix(abs, prefix) {
		return fmt.Errorf("path escapes workspace: %s", path)
	}
	return nil
}

func (c *CodeTools) rel(path string) string {
	rel, err := filepath.Rel(c.root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
