protoc-gen-go-mcp 
-----------------
This is a plugin for the [protoc compiler](https://grpc.io/docs/protoc-installation/) that generates a [model-context-protocol(MCP)](https://modelcontextprotocol.io/introduction) server based on a [protocol buffer](https://protobuf.dev/) definition. Conceptually, this allows an AI model to use existing gRPC codebases with natural language, allowing for rapid prototyping and usage of LLM capabilities for protobuf based codebases. 

#### Prerequisites
- [Go](https://go.dev/doc/install) 1.20 or later
- [protoc](https://grpc.io/docs/protoc-installation/) 3.20 or later
- [protoc-gen-go-grpc](https://grpc.io/docs/languages/go/quickstart/) 1.71 or later

#### Running the plugin
Check out the [MakeFile](./MakeFile) for explicit command usage. Use `make generate` to generate the example [proto file](./protos/example.proto) and run the example server.