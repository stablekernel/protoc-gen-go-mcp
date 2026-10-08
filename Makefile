.PHONY: protoc-gen-go-mcp generate fmt $(GOLANG_SRCS)

PROTOS := $(shell find ./examples/protos -name *.proto)

GOLANG_SRCS := $(patsubst ./%,%,$(shell find . -path "*/cmd/protoc-gen-go-mcp/*.go"))

fmt: $(GOLANG_SRCS)
	@echo "Formatting files..."
	@gofmt -s -w $(GOLANG_SRCS) && goimports -w $(GOLANG_SRCS) && echo "Formatted successfully!"

protoc-gen-go-mcp:
	@echo "Building protoc-gen-go-mcp..."
	@go build -o $(shell go env GOPATH)/bin/protoc-gen-go-mcp ./cmd/protoc-gen-go-mcp && echo "Built successful!"

generate: protoc-gen-go-mcp
	@echo "Generating new files..."
	export PATH="$(PATH):$(shell go env GOPATH)/bin"
	protoc --go_out=. $(PROTOS)
	protoc --go-grpc_out=. $(PROTOS)
	protoc --go-mcp_out=. --proto_path=examples/protos $(PROTOS) && echo "Generated successfully!"

start-debugger: protoc-gen-go-mcp
	@echo "Starting debugger"
	@protoc --plugin=protoc-gen-debug=./protoc-gen-debug --debug_out=. $(PROTOS)
