package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestUnexport(t *testing.T) {
	cases := []struct {
		name   string
		expect string
	}{
		{"", ""},
		{"F", "f"},
		{"Foo", "foo"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expect, unexport(c.name), c.name)
	}
}

func TestProcessCommentToString(t *testing.T) {
	cases := []struct {
		comments protogen.Comments
		expect   string
	}{
		{"", ""},
		{"a", "a"},
		{" foo\n", "foo"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expect, processCommentToString(c.comments), c.comments)
	}
}

func TestProtocVersion(t *testing.T) {
	zero := int32(0)
	beta := "beta"
	ver := "v0.0.0"

	cases := []struct {
		compilerVersion *pluginpb.Version
		expect          string
	}{
		{nil, "(unknown)"},
		{&pluginpb.Version{Major: &zero, Minor: &zero, Patch: &zero}, ver},
		{&pluginpb.Version{Major: &zero, Minor: &zero, Patch: &zero, Suffix: &beta}, ver + "-" + beta},
	}

	for _, c := range cases {
		gen := &protogen.Plugin{Request: &pluginpb.CodeGeneratorRequest{CompilerVersion: c.compilerVersion}}
		assert.Equal(t, c.expect, protocVersion(gen), c.compilerVersion)
	}
}

func TestGenLeadingComments(t *testing.T) {
	// The file does not get written to the filesystem by genLeadingComments, so any name is fine. Any extension
	// except .go will cause Content() to return the content of the file without parsing it as code.
	generatedFile := (&protogen.Plugin{}).NewGeneratedFile("test.txt", "")

	cases := []struct {
		loc    protoreflect.SourceLocation
		expect string
	}{
		{protoreflect.SourceLocation{}, ""},
		{
			protoreflect.SourceLocation{LeadingDetachedComments: []string{"foo"}, LeadingComments: "bar"},
			"//foo\n\n//bar\n\n",
		},
	}

	for _, c := range cases {
		genLeadingComments(generatedFile, c.loc)
		content, err := generatedFile.Content()
		require.NoError(t, err)
		assert.Equal(t, c.expect, string(content), c.loc)
	}
}

func TestGenerateMcpServerStruct(t *testing.T) {
	generatedFile := (&protogen.Plugin{}).NewGeneratedFile("test.txt", "")
	generateMcpServerStruct(generatedFile, "Server", "Client")
	content, err := generatedFile.Content()
	require.NoError(t, err)
	assert.Equal(t, "type server struct {\nClient\n\nMCPServer *server.MCPServer\n}\n\n", string(content))
}

func TestGenerateMcpServerStruct_EmptyParams(t *testing.T) {
	generatedFile := (&protogen.Plugin{}).NewGeneratedFile("test.txt", "")

	cases := []struct {
		mcpServerName string
		clientName    string
	}{
		{"", "Client"},
		{"Server", ""},
	}

	for _, c := range cases {
		assert.Panics(t, func() { generateMcpServerStruct(generatedFile, c.mcpServerName, c.clientName) }, c)
	}
}
