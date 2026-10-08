// Package examplev1_test exercises the generated MCP server end to end: a
// fake VibeService backend runs behind an in-memory gRPC connection
// (bufconn), the generated vibeServiceMCPServer sits in front of it, and the
// test talks MCP (over an in-memory client/server transport pair) to drive
// tools/list and tools/call the same way a real MCP client would.
//
// All github.com/modelcontextprotocol/go-sdk/mcp types are confined to this
// file's testHarness; every test function below works with plain types
// (toolInfo, toolResult) decoded from the raw JSON-RPC wire responses, so
// swapping the MCP client implementation again in the future only has to
// change the harness, not the tests.
package examplev1_test

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	mu sync.Mutex

	// requests records every request received, keyed by RPC name, in the
	// order received. A duplicate or missing call shows up as a slice of
	// the wrong length.
	requests map[string][]any

	// setVibeErr, if set, is returned by SetVibe instead of a response.
	setVibeErr error
}

func (f *fakeVibeService) record(name string, req any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests[name] = append(f.requests[name], req)
}

// lastRequest returns the most recently recorded request for name, or nil if
// none was recorded.
func (f *fakeVibeService) lastRequest(name string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	reqs := f.requests[name]
	if len(reqs) == 0 {
		return nil
	}
	return reqs[len(reqs)-1]
}

// callCount returns how many times name was called.
func (f *fakeVibeService) callCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests[name])
}

func (f *fakeVibeService) SetVibe(_ context.Context, req *examplev1.SetVibeRequest) (*examplev1.SetVibeResponse, error) {
	f.record("SetVibe", req)
	if f.setVibeErr != nil {
		return nil, f.setVibeErr
	}
	return &examplev1.SetVibeResponse{
		PreviousVibe: "chill",
		Vibe:         req.GetVibe(),
	}, nil
}

func (f *fakeVibeService) GetVibe(_ context.Context, req *examplev1.GetVibeRequest) (*examplev1.GetVibeResponse, error) {
	f.record("GetVibe", req)
	return &examplev1.GetVibeResponse{
		Vibe: "immaculate",
	}, nil
}

func (f *fakeVibeService) SetVibeDetails(_ context.Context, req *examplev1.SetVibeDetailsRequest) (*examplev1.SetVibeResponse, error) {
	f.record("SetVibeDetails", req)
	return &examplev1.SetVibeResponse{
		PreviousVibe: "mellow",
		Vibe:         req.GetVibe(),
	}, nil
}

// SetVibeArray returns a fixed response, independent of req, so the test can
// assert on the tool's result content without that assertion trivially
// passing no matter what the handler actually put there (see
// TestCallSetVibeArray).
func (f *fakeVibeService) SetVibeArray(_ context.Context, req *examplev1.SetVibeArrayRequest) (*examplev1.SetVibeArrayResponse, error) {
	f.record("SetVibeArray", req)
	return &examplev1.SetVibeArrayResponse{
		VibeArray: &examplev1.VibeArray{VibeBools: []bool{false}},
	}, nil
}

// SetVibeObjects returns a fixed response, independent of req, so the test
// can assert on the tool's result content without that assertion trivially
// passing no matter what the handler actually put there (see
// TestCallSetVibeObjects).
func (f *fakeVibeService) SetVibeObjects(_ context.Context, req *examplev1.SetVibeObjectsRequest) (*examplev1.SetVibeObjectsResponse, error) {
	f.record("SetVibeObjects", req)
	return &examplev1.SetVibeObjectsResponse{
		VibeObject: []*examplev1.SomeVibeObject{{Vibe: "canned"}},
	}, nil
}

// toolInfo is the MCP-client-agnostic shape of a tools/list entry: just the
// JSON an MCP client would see on the wire.
type toolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolResult is the MCP-client-agnostic shape of a tools/call result: the
// concatenated text of its content items (every RPC here returns exactly
// one text content item) and whether the call was an error.
type toolResult struct {
	Text    string
	IsError bool
}

// testHarness wires a fake gRPC backend to the generated MCP server over an
// in-memory bufconn connection, and an MCP client to that server over an
// in-memory MCP transport pair. github.com/modelcontextprotocol/go-sdk/mcp
// types are confined to this struct and its two methods (listTools,
// callTool), which decode the raw JSON results into plain types (toolInfo,
// toolResult); every test function works only with those plain types.
type testHarness struct {
	t    *testing.T
	fake *fakeVibeService
	cs   *mcp.ClientSession
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	fake := &fakeVibeService{requests: make(map[string][]any)}

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
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-vibe-service", Version: "0.0.0-test"}, nil)
	vibeMCP := examplev1.NewVibeServiceMCPServer(client, mcpServer)
	vibeMCP.RegisterDefaultTools()

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	_, err = mcpServer.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0-test"}, nil)
	cs, err := mcpClient.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	return &testHarness{t: t, fake: fake, cs: cs}
}

// listTools sends a tools/list request and returns the tools the server
// reports, in the order the server returns them.
func (h *testHarness) listTools() []toolInfo {
	h.t.Helper()

	result, err := h.cs.ListTools(context.Background(), nil)
	require.NoError(h.t, err)

	tools := make([]toolInfo, len(result.Tools))
	for i, tool := range result.Tools {
		// Round-trip the input schema through JSON rather than using the
		// SDK's *jsonschema.Schema type directly, so the rest of the test
		// only ever sees the wire format an MCP client would see.
		schemaJSON, err := json.Marshal(tool.InputSchema)
		require.NoError(h.t, err)
		var schema map[string]any
		require.NoError(h.t, json.Unmarshal(schemaJSON, &schema))

		tools[i] = toolInfo{Name: tool.Name, Description: tool.Description, InputSchema: schema}
	}
	return tools
}

// callTool sends a tools/call request for the named tool with the given
// top-level arguments and returns its result as plain text and an
// isError flag, decoded from the raw JSON-RPC wire response.
func (h *testHarness) callTool(name string, args map[string]any) toolResult {
	h.t.Helper()

	result, err := h.cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	require.NoError(h.t, err, "tools/call should not be a protocol-level error")
	require.Len(h.t, result.Content, 1, "expected exactly one content item")

	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(h.t, ok, "expected a text content item, got %T", result.Content[0])

	return toolResult{Text: text.Text, IsError: result.IsError}
}

func TestToolsList(t *testing.T) {
	h := newTestHarness(t)

	tools := h.listTools()

	names := make([]string, len(tools))
	byName := make(map[string]toolInfo, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
		byName[tool.Name] = tool
	}
	require.ElementsMatch(t, []string{
		"SetVibe", "GetVibe", "SetVibeDetails", "SetVibeArray", "SetVibeObjects",
	}, names)

	// Each tool's input schema is the top-level request message schema
	// (see cmd/protoc-gen-go-mcp/schema.go): properties are the request's
	// fields, named with their protojson (lowerCamel) JSON name, not
	// nested under a property named after the request message.
	setVibe := byName["SetVibe"]
	require.Equal(t, `This is a block comment with multiple lines to test block handling "Hello World"`, setVibe.Description)
	require.Equal(t, "object", setVibe.InputSchema["type"])
	require.Equal(t, false, setVibe.InputSchema["additionalProperties"])
	require.Equal(t, map[string]any{
		"vibe": map[string]any{
			"description": "The vibe of the server to be set",
			"type":        "string",
		},
	}, setVibe.InputSchema["properties"])
	require.NotContains(t, setVibe.InputSchema["properties"], "SetVibeRequest")

	getVibe := byName["GetVibe"]
	require.Equal(t, "Get Vibe of the server", getVibe.Description)
	require.Equal(t, "object", getVibe.InputSchema["type"])
	require.Empty(t, getVibe.InputSchema["properties"])

	setVibeDetails := byName["SetVibeDetails"]
	require.Equal(t, "Set vibe details", setVibeDetails.Description)
	require.Equal(t, "object", setVibeDetails.InputSchema["type"])
	require.Contains(t, setVibeDetails.InputSchema["properties"], "vibe")
	require.Contains(t, setVibeDetails.InputSchema["properties"], "vibeScalar")

	setVibeArray := byName["SetVibeArray"]
	require.Equal(t, "Set the vibe arrays", setVibeArray.Description)
	require.Equal(t, "object", setVibeArray.InputSchema["type"])
	require.Contains(t, setVibeArray.InputSchema["properties"], "vibeArray")

	setVibeObjects := byName["SetVibeObjects"]
	require.Equal(t, "Set multiple vibe objects", setVibeObjects.Description)
	require.Equal(t, "object", setVibeObjects.InputSchema["type"])
	require.Contains(t, setVibeObjects.InputSchema["properties"], "vibeObject")
}

func TestCallSetVibe(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibe", map[string]any{"vibe": "radical"})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibe").(*examplev1.SetVibeRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeRequest")
	require.Equal(t, "radical", req.GetVibe())
	require.Equal(t, 1, h.fake.callCount("SetVibe"))

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, "chill", body["previousVibe"])
	require.Equal(t, "radical", body["vibe"])
}

func TestCallGetVibe(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("GetVibe", map[string]any{})
	require.False(t, result.IsError)

	_, ok := h.fake.lastRequest("GetVibe").(*examplev1.GetVibeRequest)
	require.True(t, ok, "fake backend did not receive a GetVibeRequest")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, "immaculate", body["vibe"])
}

func TestCallSetVibeDetails(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeDetails", map[string]any{
		"vibe": "groovy",
		"vibeScalar": map[string]any{
			"vibeBool":   true,
			"vibeDouble": 3.5,
			"vibeInt32":  float64(42),
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibeDetails").(*examplev1.SetVibeDetailsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeDetailsRequest")
	require.Equal(t, "groovy", req.GetVibe())
	require.True(t, req.GetVibeScalar().GetVibeBool())
	require.Equal(t, 3.5, req.GetVibeScalar().GetVibeDouble())
	require.Equal(t, int32(42), req.GetVibeScalar().GetVibeInt32())

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, "groovy", body["vibe"])
}

func TestCallSetVibeArray(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeArray", map[string]any{
		"vibeArray": map[string]any{
			"vibeBools": []any{true, false, true},
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibeArray").(*examplev1.SetVibeArrayRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeArrayRequest")
	require.NotNil(t, req.GetVibeArray(), "the vibe_array message field itself should be populated")
	require.Equal(t, []bool{true, false, true}, req.GetVibeArray().GetVibeBools())

	// Assert the result content exactly, against the fake backend's fixed
	// response (see fakeVibeService.SetVibeArray), so a handler that
	// writes the wrong value (or drops the field) is caught instead of
	// just checking the key exists.
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, map[string]any{"vibeBools": []any{false}}, body["vibeArray"])
}

func TestCallSetVibeObjects(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeObjects", map[string]any{
		"vibeObject": []any{
			map[string]any{"vibe": "one"},
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibeObjects").(*examplev1.SetVibeObjectsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeObjectsRequest")
	require.Equal(t, []*examplev1.SomeVibeObject{{Vibe: "one"}}, req.GetVibeObject())

	// Assert the result content exactly, against the fake backend's fixed
	// response (see fakeVibeService.SetVibeObjects), so a handler that
	// writes the wrong value (or drops the field) is caught instead of
	// just checking the key exists.
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, []any{map[string]any{"vibe": "canned"}}, body["vibeObject"])
}

func TestCallSetVibe_GRPCError(t *testing.T) {
	h := newTestHarness(t)
	h.fake.setVibeErr = status.Error(codes.NotFound, "vibe not found")

	result := h.callTool("SetVibe", map[string]any{"vibe": "nope"})

	require.True(t, result.IsError)
	require.Contains(t, result.Text, "vibe not found")
	// The generated error handler returns err.Error() as-is (see
	// generateHandler in cmd/protoc-gen-go-mcp/mcp.go), which for a
	// status error includes "code = <Code>"; assert on the actual wire
	// text so a change to the gRPC code (not just the message) is
	// caught.
	require.Contains(t, result.Text, codes.NotFound.String())
}

func TestCallSetVibe_InvalidArgumentType(t *testing.T) {
	h := newTestHarness(t)

	// "vibe" is a string field; sending a number for it must fail schema
	// validation (performed by the SDK before the handler ever runs, see
	// mcp.AddTool in cmd/protoc-gen-go-mcp/mcp.go's generateToolRegistration)
	// rather than reach the backend.
	result := h.callTool("SetVibe", map[string]any{"vibe": 42})

	require.True(t, result.IsError)
	require.Equal(t, 0, h.fake.callCount("SetVibe"), "invalid arguments must never reach the backend")
}

func TestCallSetVibe_UnknownField(t *testing.T) {
	h := newTestHarness(t)

	// The schema sets additionalProperties: false (see MessageInputSchema
	// in cmd/protoc-gen-go-mcp/schema.go), so an unrecognized field must
	// fail schema validation rather than reach the backend.
	result := h.callTool("SetVibe", map[string]any{"vibe": "radical", "unknownField": "surprise"})

	require.True(t, result.IsError)
	require.Equal(t, 0, h.fake.callCount("SetVibe"), "invalid arguments must never reach the backend")
}
