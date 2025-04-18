protoc-gen-go-mcp 
-----------------
This is a plugin for the [protoc compiler](https://grpc.io/docs/protoc-installation/) that generates a [model-context-protocol(MCP)](https://modelcontextprotocol.io/introduction) server based on a [protocol buffer](https://protobuf.dev/) definition. Conceptually, this allows an AI model to use existing gRPC codebases with natural language, allowing for rapid prototyping and usage of LLM capabilities for protobuf based codebases. 

#### Prerequisites
- [Go](https://go.dev/doc/install) 1.20 or later
- [protoc](https://grpc.io/docs/protoc-installation/) 3.20 or later
- [protoc-gen-go-grpc](https://grpc.io/docs/languages/go/quickstart/) 1.71 or later

#### Running the plugin
Check out the [Makefile](./Makefile) for explicit command usage. Use `make generate` to generate the example MCP server from the [proto file](./protos/example.proto).

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
func (s *vibeServiceMCPServer) SetVibeTool() mcp.Tool {
	tool := mcp.NewTool("...internal instantiation of the tool")
	// ... internal implementation follows
	return tool
}
```
This tool can be subsequently registered with the server to make the RPC available to the model.

2. The `protoc-gen-go-mcp` plugin generates a default handler that interacts with a generated gRPC client for interaction with this server to parse the `mcp.Tool` into a defined gRPC request leveraging a generated client.

```golang
func (s *vibeServiceMCPServer) SetVibeHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	//... internal instantiation of the handler
}
```

3. These two pieces are combined upon registration to provide the LLM with knowledge of the RPC method and how to use them:
```golang
func (s *vibeServiceMCPServer) RegisterDefaultTools() {
	//...other tools added above
	s.MCPServer.AddTool(s.SetVibeTool(), s.SetVibeHandler)
    //...other tools added below
}
```

#### Contributing
We welcome contributions to this project! If you have an idea for a new feature or improvement, please open an issue or submit a pull request. For existing issues, assign yourself to it and start working on it. We will review your changes and provide feedback as needed.
