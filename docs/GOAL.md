# Goal

`protoc-gen-go-mcp` lets an existing gRPC service become an MCP server with
no hand-written glue: run `protoc` with this plugin, and each RPC becomes an
MCP tool whose input schema and handler come from the proto definition. AI
clients can then call the service in natural language.

## Current milestone: v0.3.0, official Go MCP SDK (#44)

Move the generated code from `github.com/mark3labs/mcp-go` to the official SDK,
**`github.com/modelcontextprotocol/go-sdk` (v1.8.0, Go 1.25)**:

- Tool input schemas are generated from the proto descriptors and describe
  exactly what `protojson.Unmarshal` accepts for the request message, because
  the official SDK validates arguments against them.
- Handlers decode arguments with `protojson`, call the gRPC client, and return
  the response as `protojson` text; gRPC errors become tool errors
  (`isError: true`).
- Golden-file and end-to-end tests land before the migration and must keep
  passing through it.
- The generated API stays as close as possible to 0.2.x; anything that changes
  is documented as a breaking change with upgrade notes.

## Out of scope for this milestone

- Streaming RPCs (still skipped by the generator).
- Arrays of messages as tool inputs (#39).
- Release binaries (#59) and vendoring (#75).
- MCP resources and prompts.
