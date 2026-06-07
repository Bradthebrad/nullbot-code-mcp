package codetools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type runner struct {
	root      string
	maxOutput int
	mu        sync.Mutex
	nextID    int
	processes map[string]*process
}

type process struct {
	id       string
	command  string
	args     []string
	cwd      string
	started  time.Time
	finished *time.Time
	exitCode *int
	err      string
	cancel   context.CancelFunc
	output   *boundedBuffer
	outputMu sync.Mutex
}

type boundedBuffer struct {
	limit     int
	truncated bool
	data      []byte
}

func newRunner(root string, maxOutput int) *runner {
	return &runner{root: root, maxOutput: maxOutput, processes: map[string]*process{}}
}

func (r *runner) start(command string, args []string, cwd string, useShell bool) (*process, error) {
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}
	r.mu.Lock()
	r.nextID++
	id := fmt.Sprintf("cmd-%d", r.nextID)
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	actualCommand := command
	actualArgs := args
	if useShell || shouldUseWindowsShell(command, args) {
		actualCommand, actualArgs = shellCommand(command)
	}
	cmd := exec.CommandContext(ctx, actualCommand, actualArgs...)
	cmd.Dir = cwd
	p := &process{
		id:      id,
		command: command,
		args:    args,
		cwd:     cwd,
		started: time.Now().UTC(),
		cancel:  cancel,
		output:  &boundedBuffer{limit: r.maxOutput},
	}
	writer := processWriter{p: p}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	r.mu.Lock()
	r.processes[id] = p
	r.mu.Unlock()
	go func() {
		err := cmd.Wait()
		now := time.Now().UTC()
		exitCode := cmd.ProcessState.ExitCode()
		p.outputMu.Lock()
		p.finished = &now
		p.exitCode = &exitCode
		if err != nil {
			p.err = err.Error()
		}
		p.outputMu.Unlock()
		cancel()
	}()
	return p, nil
}

func (r *runner) get(id string) (*process, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.processes[id]
	return p, ok
}

func shouldUseWindowsShell(command string, args []string) bool {
	if runtime.GOOS != "windows" || len(args) > 0 {
		return false
	}
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "dir", "cd", "copy", "del", "erase", "md", "mkdir", "move", "ren", "rename", "rd", "rmdir", "set", "type", "ver", "vol":
		return true
	default:
		return strings.ContainsAny(command, "|&<>")
	}
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", command}
	}
	return "sh", []string{"-c", command}
}

type processWriter struct {
	p *process
}

func (w processWriter) Write(data []byte) (int, error) {
	w.p.outputMu.Lock()
	defer w.p.outputMu.Unlock()
	w.p.output.write(data)
	return len(data), nil
}

func (b *boundedBuffer) write(data []byte) {
	if b.limit <= 0 {
		return
	}
	b.data = append(b.data, data...)
	if len(b.data) > b.limit {
		b.truncated = true
		b.data = b.data[len(b.data)-b.limit:]
	}
}

func (b *boundedBuffer) snapshot() (string, bool) {
	return string(bytes.Clone(b.data)), b.truncated
}
