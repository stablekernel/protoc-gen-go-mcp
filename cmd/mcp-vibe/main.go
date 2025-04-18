package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	examplev1 "protoc-gen-go-mcp/test/snapshots"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := newVibeServiceClient(ctx)
	mcpServer := server.NewMCPServer(
		"vibe",
		"0.0.1",
	)

	s := examplev1.NewVibeServiceMCPServer(client, mcpServer)
	s.RegisterDefaultTools()

	if err := server.ServeStdio(mcpServer); err != nil {
		fmt.Printf("failed to start server: %v\n", err)
		return
	}
}
