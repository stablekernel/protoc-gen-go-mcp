package main

// Golden-file test for the plugin's generated code.
//
// It builds a pluginpb.CodeGeneratorRequest for examples/protos/example.proto
// without running protoc: the proto file is parsed and its descriptor (with
// comments) built in-process by github.com/bufbuild/protocompile, a pure-Go
// implementation of the protobuf compiler's parsing and linking steps. (We
// cannot use protodesc.ToFileDescriptorProto on the committed example.pb.go's
// embedded descriptor for this, because protoc-gen-go strips
// SourceCodeInfo -- and with it all comments -- from the descriptor it
// embeds, to keep the compiled binary small.) The generator then runs
// against that request exactly as it would via protoc, and the resulting
// file is compared byte for byte with the committed
// examples/gen/example/v1/example_mcp.pb.go.
//
// When a deliberate change to the generator changes the output, regenerate
// the golden file with:
//
//	go test ./cmd/protoc-gen-go-mcp -run Golden -update
//
// and review the diff; an unexpected change is a bug until explained.

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/protoutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

var update = flag.Bool("update", false, "update the golden file instead of comparing against it")

const (
	// protoDir is where example.proto and anything it imports live.
	protoDir = "../../examples/protos"
	// protoFile is the file to generate, relative to protoDir.
	protoFile = "example.proto"
	// goldenFilePath is the committed file the golden test compares the
	// generator's output against: the same file `make generate` would
	// write for examples/protos/example.proto.
	goldenFilePath = "../../examples/gen/example/v1/example_mcp.pb.go"
	// compilerMajor, compilerMinor and compilerPatch pin the protoc
	// version recorded in the generated file's header comment (see the
	// "- protoc" line). They must match the protoc version used to
	// produce the committed golden file for the comparison to pass.
	compilerMajor = 5
	compilerMinor = 29
	compilerPatch = 3
)

func TestGolden(t *testing.T) {
	compiler := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: []string{protoDir}}),
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
	files, err := compiler.Compile(context.Background(), protoFile)
	require.NoError(t, err, "parsing %s", protoFile)
	require.Len(t, files, 1)

	fileDescProto := protoutil.ProtoFromFileDescriptor(files[0])
	require.Empty(t, fileDescProto.GetDependency(), "example.proto has no imports; if this changes, also add the dependencies' descriptors to ProtoFile")

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{fileDescProto.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fileDescProto},
		CompilerVersion: &pluginpb.Version{
			Major: int32Ptr(compilerMajor),
			Minor: int32Ptr(compilerMinor),
			Patch: int32Ptr(compilerPatch),
		},
	}

	gen, err := (protogen.Options{}).New(req)
	require.NoError(t, err)

	require.NoError(t, generate(gen))

	resp := gen.Response()
	require.Empty(t, resp.GetError())
	require.Len(t, resp.GetFile(), 1)

	got := resp.GetFile()[0].GetContent()

	if *update {
		require.NoError(t, os.WriteFile(goldenFilePath, []byte(got), 0o644))
		return
	}

	want, err := os.ReadFile(goldenFilePath)
	require.NoError(t, err)
	require.Equal(t, string(want), got,
		"generated output does not match %s; if this change is intentional, run `go test ./cmd/protoc-gen-go-mcp -run Golden -update`",
		goldenFilePath)
}

func int32Ptr(v int32) *int32 { return &v }
