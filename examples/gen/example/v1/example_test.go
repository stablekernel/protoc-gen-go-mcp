// This file backs the README's "What the plugin generates" and "How tool
// arguments map to the request message" sections: go vet and go build cover
// it, so the snippets shown there cannot silently drift from code that
// actually compiles against the generated API. It is not meant to be run as
// a test (ExampleWiring never returns), only compiled.
package examplev1_test

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	examplev1 "protoc-gen-go-mcp/examples/gen/example/v1"
)

// Example_wiring shows how to serve a generated MCP server over both stdio
// and Streamable HTTP, given a gRPC client for the service it wraps. It is
// compiled by `go build`/`go vet` but never run (it blocks forever), so it
// has no "Output:" comment.
func Example_wiring() {
	ctx := context.Background()

	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	client := examplev1.NewVibeServiceClient(conn)

	// Over stdio: one server, one session, for CLI-launched MCP clients.
	stdioServer := mcp.NewServer(&mcp.Implementation{Name: "vibe", Version: "0.0.1"}, nil)
	examplev1.NewVibeServiceMCPServer(client, stdioServer).RegisterDefaultTools()
	if err := stdioServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
		panic(err)
	}

	// Over Streamable HTTP: a new *mcp.Server per request (or reuse one, as
	// here, if sessions don't need to be isolated from each other).
	httpServer := mcp.NewServer(&mcp.Implementation{Name: "vibe", Version: "0.0.1"}, nil)
	examplev1.NewVibeServiceMCPServer(client, httpServer).RegisterDefaultTools()
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return httpServer
	}, nil)
	if err := http.ListenAndServe(":8080", handler); err != nil {
		panic(err)
	}
}
