// Package examplev1_test exercises the generated MCP server end to end: a
// fake VibeService backend runs behind an in-memory gRPC connection
// (bufconn), the generated vibeServiceMCPServer sits in front of it, and the
// test talks MCP (JSON-RPC over MCPServer.HandleMessage) to drive tools/list
// and tools/call the same way a real MCP client would.
//
// This test pins down what MCP clients see from the mark3labs/mcp-go-based
// generator today (#44), so the official-SDK migration (#89) can be checked
// against the same behaviour.
//
// All mark3labs/mcp-go types are confined to this file's testHarness; every
// test function below works with plain types (toolInfo, toolResult) decoded
// from the raw JSON-RPC wire responses, so swapping the MCP client for the
// official SDK (#89) only has to change the harness, not the tests.
package examplev1_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"

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
// in-memory bufconn connection. mark3labs/mcp-go types are confined to this
// struct and its two methods (listTools, callTool), which decode the raw
// JSON-RPC wire format into plain types (toolInfo, toolResult); every test
// function works only with those plain types, so a future migration to
// another MCP client (#89) only has to change this harness.
type testHarness struct {
	t     *testing.T
	fake  *fakeVibeService
	mcp   *server.MCPServer
	idSeq int
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

// handle sends a raw JSON-RPC request and returns the raw JSON of the
// "result" field of the response, failing the test if the response is not a
// successful JSON-RPC response.
func (h *testHarness) handle(req map[string]any) json.RawMessage {
	h.t.Helper()

	raw, err := json.Marshal(req)
	require.NoError(h.t, err)

	resp := h.mcp.HandleMessage(context.Background(), raw)

	// Round-trip through JSON rather than type-asserting mark3labs
	// response types, so the rest of the test only ever sees the wire
	// format an MCP client would see.
	wire, err := json.Marshal(resp)
	require.NoError(h.t, err)

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(h.t, json.Unmarshal(wire, &envelope))
	require.Nil(h.t, envelope.Error, "unexpected JSON-RPC error: %+v", envelope.Error)
	require.NotNil(h.t, envelope.Result, "expected a JSON-RPC result")

	return envelope.Result
}

// listTools sends a tools/list request and returns the tools the server
// reports, in the order the server returns them.
func (h *testHarness) listTools() []toolInfo {
	h.t.Helper()

	result := h.handle(map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID(),
		"method":  "tools/list",
	})

	var parsed struct {
		Tools []toolInfo `json:"tools"`
	}
	require.NoError(h.t, json.Unmarshal(result, &parsed))
	return parsed.Tools
}

// callTool sends a tools/call request for the named tool with the given
// top-level arguments and returns its result as plain text and an
// isError flag, decoded from the raw JSON-RPC wire response.
func (h *testHarness) callTool(name string, args map[string]any) toolResult {
	h.t.Helper()

	result := h.handle(map[string]any{
		"jsonrpc": "2.0",
		"id":      h.nextID(),
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": args,
		},
	})

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	require.NoError(h.t, json.Unmarshal(result, &parsed))
	require.Len(h.t, parsed.Content, 1, "expected exactly one content item")
	// Note: the generated error path (see generateHandler in
	// cmd/protoc-gen-go-mcp/mcp.go) does not set TextContent.Type, so
	// parsed.Content[0].Type is "" on the error path and "text" on
	// success; that's a pre-existing generator quirk, not something this
	// test asserts on.

	return toolResult{Text: parsed.Content[0].Text, IsError: parsed.IsError}
}

// knownBugT is the subset of require.TestingT that knownBug needs to drive
// require.* assertions without touching the real *testing.T.
type knownBugT struct {
	failed  bool
	message string
}

func (k *knownBugT) Errorf(format string, args ...any) {
	k.failed = true
	k.message = fmt.Sprintf(format, args...)
}

// knownBugAbort is recovered by knownBug; it exists only so FailNow can stop
// the assertion function (as require.* expects) without killing the real
// test.
type knownBugAbort struct{}

func (k *knownBugT) FailNow() {
	panic(knownBugAbort{})
}

// knownBug runs assert (typically one or more require.* calls) against a
// fake T, so a failure is expected and does not fail the real test: it logs
// the failure and moves on. If assert unexpectedly *passes*, knownBug fails
// the real test, because that means the bug tracked by issue has been
// fixed and this wrapper (and its KNOWN BUG comment) should be deleted in
// favor of asserting the correct behavior directly.
//
// This lets the official-SDK migration (#88/#89) simply delete knownBug
// wrappers as it fixes the underlying bugs, rather than updating assertions
// on buggy values throughout the test.
func knownBug(t *testing.T, issue string, assert func(r require.TestingT)) {
	t.Helper()

	kt := &knownBugT{}
	func() {
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(knownBugAbort); !ok {
					panic(p)
				}
			}
		}()
		assert(kt)
	}()

	if kt.failed {
		t.Logf("KNOWN BUG (%s), still present as expected: %s", issue, kt.message)
		return
	}
	t.Errorf("KNOWN BUG (%s) appears to be fixed; delete this knownBug wrapper and assert the correct behavior directly", issue)
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

	// Snapshot what's correct today: tool name, description, and the
	// schema's top-level type. These are the contract MCP clients see;
	// the official-SDK migration (#89) must keep producing equivalent
	// values.
	//
	// KNOWN BUG (#88/#89): the schema nests each tool's properties under
	// a property named after the request message (e.g. "SetVibeRequest"),
	// but the handler (see below) reads arguments from the top level of
	// the arguments map. Don't assert the nested property name as
	// correct; the migration should replace it with top-level properties
	// matching the request fields.
	setVibe := byName["SetVibe"]
	require.Equal(t, `This is a block comment with multiple lines to test block handling "Hello World"`, setVibe.Description)
	require.Equal(t, "object", setVibe.InputSchema["type"])

	getVibe := byName["GetVibe"]
	require.Equal(t, "Get Vibe of the server", getVibe.Description)
	require.Equal(t, "object", getVibe.InputSchema["type"])
	require.Empty(t, getVibe.InputSchema["properties"])

	setVibeDetails := byName["SetVibeDetails"]
	require.Equal(t, "Set vibe details", setVibeDetails.Description)
	require.Equal(t, "object", setVibeDetails.InputSchema["type"])

	setVibeArray := byName["SetVibeArray"]
	require.Equal(t, "Set the vibe arrays", setVibeArray.Description)
	require.Equal(t, "object", setVibeArray.InputSchema["type"])

	setVibeObjects := byName["SetVibeObjects"]
	require.Equal(t, "Set multiple vibe objects", setVibeObjects.Description)
	require.Equal(t, "object", setVibeObjects.InputSchema["type"])
}

func TestCallSetVibe(t *testing.T) {
	h := newTestHarness(t)

	// KNOWN BUG (#88/#89): see TestToolsList for the schema/handler
	// mismatch. Exercise what the handler does today (top-level
	// arguments), not what the (wrong) schema advertises.
	result := h.callTool("SetVibe", map[string]any{"vibe": "radical"})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibe").(*examplev1.SetVibeRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeRequest")
	require.Equal(t, "radical", req.GetVibe())
	require.Equal(t, 1, h.fake.callCount("SetVibe"))

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, "chill", body["previous_vibe"])
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
		"vibe_scalar": map[string]any{
			"vibe_bool":   true,
			"vibe_double": 3.5,
			"vibe_int32":  float64(42),
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibeDetails").(*examplev1.SetVibeDetailsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeDetailsRequest")
	require.Equal(t, "groovy", req.GetVibe())
	require.True(t, req.GetVibeScalar().GetVibeBool())
	require.Equal(t, 3.5, req.GetVibeScalar().GetVibeDouble())

	// KNOWN BUG (#88/#89): the generated nested-field assignment for
	// int32 asserts the decoded JSON value is a Go `int32` (see
	// generateFieldAssignment's default case in
	// cmd/protoc-gen-go-mcp/mcp.go), but encoding/json always decodes
	// JSON numbers into `map[string]any` as float64. That assertion
	// always fails, so vibe_int32 is silently dropped even though it was
	// sent above. Assert the *correct* value here: once the migration
	// fixes the decoding, this wrapper starts failing and should be
	// deleted in favor of `require.Equal(t, int32(42), ...)` directly.
	knownBug(t, "#88/#89", func(r require.TestingT) {
		require.Equal(r, int32(42), req.GetVibeScalar().GetVibeInt32())
	})

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
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

	req, ok := h.fake.lastRequest("SetVibeArray").(*examplev1.SetVibeArrayRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeArrayRequest")
	require.NotNil(t, req.GetVibeArray(), "the vibe_array message field itself should be populated")

	// KNOWN BUG (#88/#89): the generated repeated-scalar assignment
	// asserts the decoded JSON value is a concrete Go slice (`[]bool`
	// here), but encoding/json always decodes JSON arrays into
	// `map[string]any` as `[]any`. That assertion always fails, so
	// repeated scalar fields are silently dropped regardless of what the
	// client sends. Assert the *correct* value here: once fixed, this
	// wrapper starts failing and should be deleted in favor of asserting
	// the slice directly.
	knownBug(t, "#88/#89", func(r require.TestingT) {
		require.Equal(r, []bool{true, false, true}, req.GetVibeArray().GetVibeBools())
	})

	// Assert the result content exactly, against the fake backend's fixed
	// response (see fakeVibeService.SetVibeArray), so a handler that
	// writes the wrong value (or drops the field) is caught instead of
	// just checking the key exists.
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, map[string]any{"vibe_bools": []any{false}}, body["vibe_array"])
}

func TestCallSetVibeObjects(t *testing.T) {
	h := newTestHarness(t)

	result := h.callTool("SetVibeObjects", map[string]any{
		"vibe_object": []any{
			map[string]any{"vibe": "one"},
		},
	})
	require.False(t, result.IsError)

	req, ok := h.fake.lastRequest("SetVibeObjects").(*examplev1.SetVibeObjectsRequest)
	require.True(t, ok, "fake backend did not receive a SetVibeObjectsRequest")

	// KNOWN BUG (#39): the generator does not currently populate
	// repeated-message fields (see generateHandler's "message" + IsList
	// case), so the fake backend receives an empty request regardless of
	// arguments. Assert the *correct* value here: once #39 adds support,
	// this wrapper starts failing and should be deleted in favor of
	// asserting the populated slice directly.
	knownBug(t, "#39", func(r require.TestingT) {
		require.Equal(r, []*examplev1.SomeVibeObject{{Vibe: "one"}}, req.GetVibeObject())
	})

	// Assert the result content exactly, against the fake backend's fixed
	// response (see fakeVibeService.SetVibeObjects), so a handler that
	// writes the wrong value (or drops the field) is caught instead of
	// just checking the key exists.
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.Text), &body))
	require.Equal(t, []any{map[string]any{"vibe": "canned"}}, body["vibe_object"])
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
