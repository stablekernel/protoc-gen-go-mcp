package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	examplev1 "protoc-gen-go-mcp/examples/gen/example/v1"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := newVibeServiceClient(ctx)
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "vibe",
		Version: "0.0.1",
	}, nil)

	s := examplev1.NewVibeServiceMCPServer(client, mcpServer)
	s.RegisterDefaultTools()

	if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
		fmt.Printf("failed to start server: %v\n", err)
		return
	}
}
