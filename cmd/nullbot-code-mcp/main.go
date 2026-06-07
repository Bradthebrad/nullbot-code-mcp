package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Bradthebrad/nullbot-code-mcp/internal/codetools"
	"tinychain/mcp"
)

const version = "0.1.0"

func main() {
	transport := flag.String("transport", "stdio", "Transport: stdio, streamable-http, http, or sse.")
	addr := flag.String("addr", "127.0.0.1:8765", "HTTP/SSE listen address.")
	path := flag.String("path", "/mcp", "Streamable HTTP endpoint path.")
	ssePath := flag.String("sse-path", "/sse", "Legacy SSE endpoint path.")
	messagePath := flag.String("message-path", "/message", "Legacy SSE message endpoint path.")
	workspace := flag.String("workspace", ".", "Workspace root. File tools cannot escape this directory.")
	maxReadBytes := flag.Int64("max-read-bytes", 512*1024, "Maximum bytes returned by read_file.")
	maxCommandBytes := flag.Int("max-command-bytes", 128*1024, "Maximum command output bytes retained per command.")
	showVersion := flag.Bool("version", false, "Print version and exit.")
	flag.Parse()

	if *showVersion {
		fmt.Println("nullbot-code-mcp", version)
		return
	}

	codeTools, err := codetools.New(codetools.Config{
		Workspace:       *workspace,
		MaxReadBytes:    *maxReadBytes,
		MaxCommandBytes: *maxCommandBytes,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "nullbot-code-mcp:", err)
		os.Exit(2)
	}

	server := mcp.NewServer("nullbot-code-mcp")
	server.Version = version
	for _, tool := range codeTools.Tools() {
		server.AddTool(tool)
	}

	if *transport != "stdio" {
		fmt.Fprintf(os.Stderr, "nullbot-code-mcp serving %s on %s\n", *transport, *addr)
	}
	err = server.Run(
		context.Background(),
		mcp.WithTransport(*transport),
		mcp.WithAddr(*addr),
		mcp.WithPath(*path),
		mcp.WithSSEPaths(*ssePath, *messagePath),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "nullbot-code-mcp:", err)
		os.Exit(1)
	}
}
