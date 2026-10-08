// Package examplev1_test exercises the generated MCP server end to end: a
// fake VibeService backend runs behind an in-memory gRPC connection
// (bufconn), the generated vibeServiceMCPServer sits in front of it, and the
// test talks MCP (JSON-RPC over MCPServer.HandleMessage) to drive tools/list
// and tools/call the same way a real MCP client would.
//
// This test pins down what MCP clients see from the mark3labs/mcp-go-based
// generator today (#44), so the official-SDK migration (#89) can be checked
// against the same behaviour.
package examplev1_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	examplev1 "protoc-gen-go-mcp/examples/gen/example/v1"
)

// fakeVibeService is a hand-written VibeServiceServer that records the
// requests it receives and returns canned responses or errors, so the test
// can assert on exactly what the generated MCP handler sent it.
type fakeVibeService struct {
	examplev1.UnimplementedVibeServiceServer

	// requests records every request received, keyed by RPC name.
	requests map[string]any

	// setVibeErr, if set, is returned by SetVibe instead of a response.
	setVibeErr error
}

func (f *fakeVibeService) SetVibe(_ context.Context, req *examplev1.SetVibeRequest) (*examplev1.SetVibeResponse, error) {
	f.requests["SetVibe"] = req
	if f.setVibeErr != nil {
		return nil, f.setVibeErr
	}
	return &examplev1.SetVibeResponse{
		PreviousVibe: "chill",
		Vibe:         req.GetVibe(),
	}, nil
}

func (f *fakeVibeService) GetVibe(_ context.Context, req *examplev1.GetVibeRequest) (*examplev1.GetVibeResponse, error) {
	f.requests["GetVibe"] = req
	return &examplev1.GetVibeResponse{
		Vibe: "immaculate",
	}, nil
}

func (f *fakeVibeService) SetVibeDetails(_ context.Context, req *examplev1.SetVibeDetailsRequest) (*examplev1.SetVibeResponse, error) {
	f.requests["SetVibeDetails"] = req
	return &examplev1.SetVibeResponse{
		PreviousVibe: "mellow",
		Vibe:         req.GetVibe(),
	}, nil
}

func (f *fakeVibeService) SetVibeArray(_ context.Context, req *examplev1.SetVibeArrayRequest) (*examplev1.SetVibeArrayResponse, error) {
	f.requests["SetVibeArray"] = req
	return &examplev1.SetVibeArrayResponse{
		VibeArray: req.GetVibeArray(),
	}, nil
}

func (f *fakeVibeService) SetVibeObjects(_ context.Context, req *examplev1.SetVibeObjectsRequest) (*examplev1.SetVibeObjectsResponse, error) {
	f.requests["SetVibeObjects"] = req
	return &examplev1.SetVibeObjectsResponse{
		VibeObject: req.GetVibeObject(),
	}, nil
}

// testHarness wires a fake gRPC backend to the generated MCP server over an
// in-memory bufconn connection, and exposes small MCP-client helpers
// (listTools, callTool) so a future migration to another MCP client only has
// to change those two helpers.
type testHarness struct {
	t     *testing.T
	fake  *fakeVibeService
	mcp   *server.MCPServer
	idSeq int
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	fake := &fakeVibeService{requests: make(map[string]any)}

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	examplev1.RegisterVibeServiceServer(grpcServer, fake)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(grpcServer.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := examplev1.NewVibeServiceClient(conn)
	mcpServer := server.NewMCPServer("test-vibe-service", "0.0.0-test")
	vibeMCP := examplev1.NewVibeServiceMCPServer(client, mcpServer)
	vibeMCP.RegisterDefaultTools()

	return &testHarness{t: t, fake: fake, mcp: mcpServer}
}

// nextID returns a fresh JSON-RPC request id.
func (h *testHarness) nextID() int {
	h.idSeq++
	return h.idSeq
}

// listTools sends a tools/list request and returns the tools the server
// reports, in the order the server returns them.
func (h *testHarness) listTools() []mcp.Tool {
	h.t.Helper()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID(),
		"method":  "tools/list",
	}
	raw, err := json.Marshal(req)
	require.NoError(h.t, err)

	resp := h.mcp.HandleMessage(context.Background(), raw)
	jsonResp, ok := resp.(mcp.JSONRPCResponse)
	require.True(h.t, ok, "expected a JSONRPCResponse, got %T: %+v", resp, resp)

	result, ok := jsonResp.Result.(mcp.ListToolsResult)
	require.True(h.t, ok, "expected a ListToolsResult, got %T", jsonResp.Result)

	return result.Tools
}

// callTool sends a tools/call request for the named tool with the given
// top-level arguments and returns the CallToolResult.
func (h *testHarness) callTool(name string, args map[string]any) *mcp.CallToolResult {
	h.t.Helper()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	}
	raw, err := json.Marshal(req)
	require.NoError(h.t, err)

	resp := h.mcp.HandleMessage(context.Background(), raw)
	jsonResp, ok := resp.(mcp.JSONRPCResponse)
	require.True(h.t, ok, "expected a JSONRPCResponse, got %T: %+v", resp, resp)

	result, ok := jsonResp.Result.(mcp.CallToolResult)
	require.True(h.t, ok, "expected a CallToolResult, got %T", jsonResp.Result)

	return &result
}

// textContent returns the concatenated text of a CallToolResult's text
// content items, for convenience in assertions.
func textContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1, "expected exactly one content item")
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected a *TextContent, got %T", result.Content[0])
	return tc.Text
}

func TestToolsList(t *testing.T) {
	h := newTestHarness(t)

	tools := h.listTools()

	names := make([]string, len(tools))
	byName := make(map[string]mcp.Tool, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
		byName[tool.Name] = tool
	}
	require.ElementsMatch(t, []string{
		"SetVibe", "GetVibe", "SetVibeDetails", "SetVibeArray", "SetVibeObjects",
	}, names)

	// Snapshot the description and input schema for each tool. These are
	// the contract MCP clients see; the official-SDK migration (#89) must
	// keep producing equivalent schemas (modulo the KNOWN BUG noted below).
	setVibe := byName["SetVibe"]
	require.Equal(t, `This is a block comment with multiple lines to test block handling "Hello World"`, setVibe.Description)
	require.Equal(t, "object", setVibe.InputSchema.Type)
	// KNOWN BUG (#88/#89): the schema nests the "vibe" property under a
	// "SetVibeRequest" object, but the handler (see below) reads "vibe"
	// from the top level of the arguments map. Document the schema as
	// generated today without asserting it matches what the handler
	// actually reads.
	require.Contains(t, setVibe.InputSchema.Properties, "SetVibeRequest")

	getVibe := byName["GetVibe"]
	require.Equal(t, "Get Vibe of the server", getVibe.Description)
	require.Equal(t, "object", getVibe.InputSchema.Type)
	require.Empty(t, getVibe.InputSchema.Properties)

	setVibeDetails := byName["SetVibeDetails"]
	require.Equal(t, "Set vibe details", setVibeDetails.Description)
	require.Contains(t, setVibeDetails.InputSchema.Properties, "SetVibeDetailsRequest")

	setVibeArray := byName["SetVibeArray"]
	require.Equal(t, "Set the vibe arrays", setVibeArray.Description)
	require.Contains(t, setVibeArray.InputSchema.Properties, "SetVibeArrayRequest")

	setVibeObjects := byName["SetVibeObjects"]
	require.Equal(t, "Set multiple vibe objects", setVibeObjects.Description)
	require.Contains(t, setVibeObjects.InputSchema.Properties, "SetVibeObjectsRequest")
}

func TestCallSetVibe(t *testing.T) {
	h := newTestHarness(t)

	// KNOWN BUG (#88/#89): the generated schema nests "vibe" under a
	// "SetVibeRequest" object property, but the handler reads arguments
	// from the top level of req.Params.Arguments. Exercise what the
	// handler does today (top-level arguments).
	result := h.callTool("SetVibe", map[string]any{"vibe": "radical"})
	require.False(t, result.IsError)

	req, ok := h.fake.requests["SetVibe"].(*examplev1.SetVibeRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeRequest")
	require.Equal(t, "radical", req.GetVibe())

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent(t, result)), &body))
	require.Equal(t, "chill", body["previous_vibe"])
	require.Equal(t, "radical", body["vibe"])
}

func TestCallGetVibe(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("GetVibe", map[string]any{})
	require.False(t, result.IsError)

	_, ok := h.fake.requests["GetVibe"].(*examplev1.GetVibeRequest)
	require.True(t, ok, "fake backend did not receive a GetVibeRequest")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent(t, result)), &body))
	require.Equal(t, "immaculate", body["vibe"])
}

func TestCallSetVibeDetails(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeDetails", map[string]any{
		"vibe": "groovy",
		"vibe_scalar": map[string]any{
			"vibe_bool":   true,
			"vibe_double": 3.5,
			// KNOWN BUG: the generated nested-field assignment for
			// int32 asserts the decoded JSON value is a Go `int32`
			// (see generateFieldAssignment's default case in
			// cmd/protoc-gen-go-mcp/mcp.go), but encoding/json always
			// decodes JSON numbers into `map[string]any` as float64.
			// That assertion always fails, so vibe_int32 is silently
			// dropped. Send it anyway to document that it has no
			// effect today.
			"vibe_int32": float64(42),
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.requests["SetVibeDetails"].(*examplev1.SetVibeDetailsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeDetailsRequest")
	require.Equal(t, "groovy", req.GetVibe())
	require.True(t, req.GetVibeScalar().GetVibeBool())
	require.Equal(t, 3.5, req.GetVibeScalar().GetVibeDouble())
	require.Equal(t, int32(0), req.GetVibeScalar().GetVibeInt32(), "KNOWN BUG: int32 nested fields are never populated, see comment above")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(textContent(t, result)), &body))
	require.Equal(t, "groovy", body["vibe"])
}

func TestCallSetVibeArray(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeArray", map[string]any{
		"vibe_array": map[string]any{
			"vibe_bools": []any{true, false, true},
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.requests["SetVibeArray"].(*examplev1.SetVibeArrayRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeArrayRequest")
	// KNOWN BUG: the generated repeated-scalar assignment asserts the
	// decoded JSON value is a concrete Go slice (`[]bool` here), but
	// encoding/json always decodes JSON arrays into `map[string]any` as
	// `[]any`. That assertion always fails, so repeated scalar fields are
	// silently dropped regardless of what the client sends.
	require.Empty(t, req.GetVibeArray().GetVibeBools(), "KNOWN BUG: repeated scalar fields are never populated, see comment above")
}

func TestCallSetVibeObjects(t *testing.T) {
	h := newTestHarness(t)

	// The generator does not currently populate repeated-message fields
	// (see generateHandler's "message" + IsList case, and #39), so the
	// fake backend receives an empty request regardless of arguments.
	result := h.callTool("SetVibeObjects", map[string]any{
		"vibe_object": []any{
			map[string]any{"vibe": "one"},
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.requests["SetVibeObjects"].(*examplev1.SetVibeObjectsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeObjectsRequest")
	require.Empty(t, req.GetVibeObject())
}

func TestCallSetVibe_GRPCError(t *testing.T) {
	h := newTestHarness(t)
	h.fake.setVibeErr = status.Error(codes.NotFound, "vibe not found")

	result := h.callTool("SetVibe", map[string]any{"vibe": "nope"})

	require.True(t, result.IsError)
	require.Contains(t, textContent(t, result), "vibe not found")
}

// TestDeadlineSanity keeps this file honest about the "under 5 seconds, no
// network, no protoc" requirement: it exercises the full stack once more
// with a short deadline so a regression that blocks on real I/O fails fast
// instead of hanging the suite.
func TestDeadlineSanity(t *testing.T) {
	h := newTestHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "GetVibe",
			"arguments": map[string]any{},
		},
	}
	raw, err := json.Marshal(req)
	require.NoError(t, err)

	resp := h.mcp.HandleMessage(ctx, raw)
	_, ok := resp.(mcp.JSONRPCResponse)
	require.True(t, ok)
}
