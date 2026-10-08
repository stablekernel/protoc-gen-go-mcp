protoc-gen-go-mcp 
-----------------
This is a [Topeka](#topeka) plugin for the [protoc compiler](https://grpc.io/docs/protoc-installation/) that generates a [model-context-protocol(MCP)](https://modelcontextprotocol.io/introduction) server based on a [protocol buffer](https://protobuf.dev/) definition. Conceptually, this allows an AI model to use existing [gRPC](https://grpc.io/) codebases with natural language, allowing for rapid prototyping and usage of LLM capabilities for protobuf based codebases.

#### Prerequisites
- [Go](https://go.dev/doc/install) 1.25 or later (the generated code imports
  the official MCP SDK, which requires it)
- [protoc](https://grpc.io/docs/protoc-installation/) 29.3 (the version this
  plugin is tested against; see [`Makefile`](./Makefile) and
  [`AGENTS.md`](./AGENTS.md))
- [protoc-gen-go](https://pkg.go.dev/google.golang.org/protobuf/cmd/protoc-gen-go)
  v1.36.6 and
  [protoc-gen-go-grpc](https://pkg.go.dev/google.golang.org/grpc/cmd/protoc-gen-go-grpc)
  v1.5.1 (the exact versions pinned as `tool` directives in
  [`go.mod`](./go.mod))

#### Running the plugin
Check out the [Makefile](./Makefile) for explicit command usage. Use `make generate` to generate the example MCP server from the [proto file](./examples/protos/example.proto).

#### Debugging the plugin

```bash
make start-debugger
```

Will start the delve debugger in headless mode on port 2345. The file under
debug will be `$GOPATH/bin/protoc-gen-go-mcp`. You can connect to that in VSCode
with the following `launch.json` configuration.

```json
{
  "name": "Connect to protoc-gen-go-mcp plugin",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "remotePath": "${workspaceFolder}",
  "port": 2345,
  "host": "127.0.0.1"
}
```

#### Testing the example
Install the example `mcp-vibe` server
```bash
go install ./cmd/mcp-vibe
```

Add the `mcp-vibe` server to your mcp servers:
```json
{
  "mcpServers": {
    "vibe": {
      "command": "mcp-vibe"
    }
  }
}
```

#### What the plugin generates
A generated `New<Service>MCPServer` wraps a gRPC client and an
[`*mcp.Server`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#Server)
from the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk).
Once you've called `RegisterDefaultTools()` (or registered individual tools,
see below), serve that `*mcp.Server` the way any MCP server is served. The
two most common transports:

```golang
// client is a examplev1.VibeServiceClient for the backend gRPC service, e.g.
// examplev1.NewVibeServiceClient(conn) for a grpc.ClientConn.

// Over stdio, for CLI-launched MCP clients (see cmd/mcp-vibe/main.go for the
// full, runnable version of this).
server := mcp.NewServer(&mcp.Implementation{Name: "vibe", Version: "0.0.1"}, nil)
examplev1.NewVibeServiceMCPServer(client, server).RegisterDefaultTools()
if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
	log.Fatal(err)
}
```

```golang
// Over Streamable HTTP, for network clients. client is the same
// examplev1.VibeServiceClient as above.
server := mcp.NewServer(&mcp.Implementation{Name: "vibe", Version: "0.0.1"}, nil)
examplev1.NewVibeServiceMCPServer(client, server).RegisterDefaultTools()
handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
	return server
}, nil)
log.Fatal(http.ListenAndServe(":8080", handler))
```

Both snippets are kept compiling in
[`examples/gen/example/v1/example_test.go`](./examples/gen/example/v1/example_test.go)
(`Example_wiring`), so they stay in sync with the generated API; `go vet` and
`go build` cover that file on every run of the [checks](./AGENTS.md).

#### How tool arguments map to the request message
Each tool's `InputSchema` is the [JSON Schema](https://json-schema.org/) for
its RPC's request message, and the generated handler decodes incoming
arguments with
[`protojson.Unmarshal`](https://pkg.go.dev/google.golang.org/protobuf/encoding/protojson#Unmarshal).
That means tool arguments follow protojson's mapping, not the proto field
names or Go struct tags:

- **Field names are protojson names** (lowerCamel by default, e.g. a proto
  field `previous_vibe` is the JSON key `previousVibe`), not the original
  snake_case proto name. The schema's `additionalProperties: false` rejects
  an unrecognized key — including the snake_case form — as a tool error
  rather than silently ignoring or coercing it.
- **64-bit integers (`int64`, `uint64`, `sint64`, `fixed64`, `sfixed64`) are
  accepted as either a JSON number or a decimal string** (`"type":
  ["integer", "string"]`), matching protojson's own encoding of them as
  strings (to avoid precision loss in JSON's float64 number type). All other
  integer kinds are plain JSON numbers.
- **Enum fields are their value's name as a string** (e.g. `"VIBE_GOOD"`),
  not the underlying numeric value, matching protojson's default enum
  encoding.
- **`bytes` fields are base64-encoded strings**, per protojson's `bytes`
  mapping.
- Tool call results are protojson-encoded the same way, so a result's keys
  and enum/int64 representations follow the same rules.

#### Philosophical Notes 
The plugin uses the existing code generation for protocol buffers and gRPC servers and builds upon that base, using and reusing parts where necessary. This gives us a healthy amount of code reuse while allowing us to control what we expose to end users. We want this plugin to provide sane, out-of-the-box functionality while allowing for easy extension.

How is this achieved? 

The code is broken into composable parts: 

1. The `protoc-gen-go-mcp` plugin generates [default tools](https://modelcontextprotocol.io/docs/concepts/tools) based on the request parameters for any given RPC.
eg: 
```proto
message SetVibeRequest {
  string vibe = 1;
}

message SetVibeResponse {
  string previous_vibe = 1;
  string vibe = 2;
}

service VibeService {
  // Set Vibe
  rpc SetVibe(SetVibeRequest) returns (SetVibeResponse) {}
}
```
This snippet defines the `SetVibe` RPC, which takes a `SetVibeRequest` message and contains a definition of the request parameter `SetVibeRequest` message. The plugin generates the following tools by default:
```golang
func (s *vibeServiceMCPServer) SetVibeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "SetVibe",
		Description: "Set Vibe",
		InputSchema: json.RawMessage(`{"type":"object", ...}`), // JSON Schema derived from SetVibeRequest
	}
}
```
This tool can be subsequently registered with the server to make the RPC available to the model.

2. The `protoc-gen-go-mcp` plugin generates a default handler that interacts with a generated gRPC client for interaction with this server to parse the `mcp.Tool` into a defined gRPC request leveraging a generated client.

```golang
func (s *vibeServiceMCPServer) SetVibeHandler(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error) {
	//... internal instantiation of the handler
}
```

3. These two pieces are combined upon registration to provide the LLM with knowledge of the RPC method and how to use them. Registration uses [`mcp.AddTool`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#AddTool), so the SDK validates incoming arguments against the tool's input schema before the handler ever runs:
```golang
func (s *vibeServiceMCPServer) RegisterDefaultTools() {
	//...other tools added above
	s.RegisterTool(s.SetVibeTool(), s.SetVibeHandler)
    //...other tools added below
}
```

#### Upgrading from 0.2.x
0.3.0 moves generated code from
[`github.com/mark3labs/mcp-go`](https://github.com/mark3labs/mcp-go) to the
[official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
(`github.com/modelcontextprotocol/go-sdk`, requiring Go 1.25). Regenerating
with 0.3.0 changes the generated API:

- **`New<Service>MCPServer` takes a
  [`*mcp.Server`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#Server)**
  (was `*server.MCPServer`), and the generated struct's exported `MCPServer`
  field has the same new type. Construct it with `mcp.NewServer(&mcp.Implementation{...}, nil)`
  and serve it as shown in [What the plugin generates](#what-the-plugin-generates)
  above.
- **`XxxTool()` returns `*mcp.Tool`** (was `mcp.Tool`), built directly as
  `&mcp.Tool{Name, Description, InputSchema}` rather than via `mcp.NewTool(...)`.
- **`RegisterTool`'s signature changed**: its tool parameter is now `*mcp.Tool`
  (was `mcp.Tool`) and its handler parameter is now
  `mcp.ToolHandlerFor[json.RawMessage, any]` — i.e.
  `func(ctx context.Context, req *mcp.CallToolRequest, args json.RawMessage) (*mcp.CallToolResult, any, error)`
  (was `func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)`).
  Registration now goes through
  [`mcp.AddTool`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#AddTool),
  so the SDK validates incoming arguments against the tool's input schema
  before the handler ever runs — the old SDK didn't enforce the schema at
  all.
- **Each tool's `InputSchema` is now the top-level request-message schema**
  (previously nested under a property named after the request message).
- **Tool call arguments and results are protojson**, not the old
  reflection-based encoding: see
  [How tool arguments map to the request message](#how-tool-arguments-map-to-the-request-message)
  above for the field-naming, 64-bit-integer, and enum differences this
  implies. In particular, a caller sending the old snake_case field names
  (e.g. `previous_vibe`) now gets a tool error, because the schema's
  `additionalProperties: false` rejects unrecognized keys.
- **Go 1.25 or later is required** to build generated code and this plugin.

To upgrade: update Go, `protoc-gen-go`, and `protoc-gen-go-grpc` to the
versions in [Prerequisites](#prerequisites); regenerate with
`protoc --go-mcp_out=...` (or `make generate` for the example); update any
callers that construct the generated server, call `RegisterTool` directly,
or send/parse tool arguments or results, per the changes above.

#### Topeka
[Topeka](https://topeka.ai) is an open source project that provides code-generators for [Model-Context-Protocol (MCP)](https://modelcontextprotocol.io/introduction).

It is designed to facilitate the usage of MCP seamlessly against existing gRPC based applications. This is done via
leveraging code generation using the [protoc compiler](https://grpc.io/docs/protoc-installation/) and installing the relevant Topeka plugin.

The plugins follow [Semantic Versioning](https://semver.org/) and any plugin prior to 1.0.0 releases ARE still subject to breaking changes. Please note, this is
applied to the generated servers, not the plugins themselves, which do not provide public APIs. This project reserves the right to change how code generation is achieved,
while maintaining stable MCP server APIs.

#### Maintainers
[Stable Kernel](https://stablekernel.com) is the primary maintainer of this project and sponsor of the plugins, though we welcome outside contributions.

[Stable Kernel](https://stablekernel.com) is a digital transformation company building solutions that power LLM enablement for growing businesses. We have a track record of helping our partners solve their biggest challenges on their digital journey, whether they need insights or implementation. Every day, millions of people rely on software that we developed, and our custom software development and technology services have been trusted by some of the most innovative Fortune 500 companies in the world. 
