package main

import (
	_ "embed"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"google.golang.org/protobuf/types/pluginpb"

	schemapb "protoc-gen-go-mcp/cmd/protoc-gen-go-mcp/testdata/schemapb"
)

// schemaProtoSet is a pre-built google.protobuf.FileDescriptorSet (with
// --include_source_info, so leading comments are present) for schema.proto
// and its transitive imports. It is generated with:
//
//	protoc --proto_path=. --proto_path=<well-known-types include dir> \
//	  --include_source_info --include_imports \
//	  --descriptor_set_out=cmd/protoc-gen-go-mcp/testdata/schemapb/schema.binpb \
//	  cmd/protoc-gen-go-mcp/testdata/schemapb/schema.proto
//
// It is checked in (rather than built by the test, or via
// protodesc.ToFileDescriptorProto on the registered descriptors) because the
// registered descriptors only carry runtime-retention options and do not
// include comments, which the schema builder's descriptions depend on.
//
//go:embed testdata/schemapb/schema.binpb
var schemaProtoSet []byte

// loadMessage builds the *protogen.Message for a message type declared in
// schema.proto, by feeding protogen a CodeGeneratorRequest built from
// schemaProtoSet. This mirrors what protoc actually sends a plugin.
func loadMessage(t *testing.T, messageName protoreflect.Name) *protogen.Message {
	t.Helper()

	var fds descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(schemaProtoSet, &fds))

	const path = "cmd/protoc-gen-go-mcp/testdata/schemapb/schema.proto"
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{path},
		ProtoFile:      fds.File,
	}
	plugin, err := protogen.Options{}.New(req)
	require.NoError(t, err)

	file := plugin.FilesByPath[path]
	require.NotNil(t, file)

	for _, m := range file.Messages {
		if m.Desc.Name() == messageName {
			return m
		}
	}
	t.Fatalf("message %q not found in file %q", messageName, path)
	return nil
}

// withDescription returns a copy of schema with "description" set to desc,
// for building the "want" side of a table-driven assertion from a schema
// returned by a production helper like floatSchema.
func withDescription(schema JSONSchema, desc string) JSONSchema {
	out := JSONSchema{"description": desc}
	for k, v := range schema {
		out[k] = v
	}
	return out
}

func TestMessageInputSchema_AllScalars(t *testing.T) {
	msg := loadMessage(t, "AllScalars")
	schema := MessageInputSchema(msg)

	props, ok := schema["properties"].(JSONSchema)
	require.True(t, ok, "properties should be a JSONSchema map")

	cases := []struct {
		field string
		want  JSONSchema
	}{
		{"sString", JSONSchema{"type": "string", "description": "A string field."}},
		{"sBytes", JSONSchema{"type": "string", "contentEncoding": "base64", "description": "A bytes field."}},
		{"sBool", JSONSchema{"type": "boolean", "description": "A bool field."}},
		{"sInt32", JSONSchema{"type": "integer", "description": "An int32 field."}},
		{"sSint32", JSONSchema{"type": "integer", "description": "A sint32 field."}},
		{"sSfixed32", JSONSchema{"type": "integer", "description": "An sfixed32 field."}},
		{"sUint32", JSONSchema{"type": "integer", "minimum": 0, "description": "A uint32 field."}},
		{"sFixed32", JSONSchema{"type": "integer", "minimum": 0, "description": "A fixed32 field."}},
		{"sInt64", JSONSchema{"type": []any{"integer", "string"}, "description": "An int64 field."}},
		{"sSint64", JSONSchema{"type": []any{"integer", "string"}, "description": "A sint64 field."}},
		{"sSfixed64", JSONSchema{"type": []any{"integer", "string"}, "description": "An sfixed64 field."}},
		{"sUint64", JSONSchema{"type": []any{"integer", "string"}, "minimum": 0, "description": "A uint64 field."}},
		{"sFixed64", JSONSchema{"type": []any{"integer", "string"}, "minimum": 0, "description": "A fixed64 field."}},
		{"sFloat", withDescription(floatSchema(), "A float field.")},
		{"sDouble", withDescription(floatSchema(), "A double field.")},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			assert.Equal(t, c.want, props[c.field], "schema for field %q", c.field)
		})
	}

	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.NotContains(t, schema, "required", "proto3 messages must not emit \"required\"")
	assert.Equal(t, "AllScalars covers every scalar proto kind in the schema table.", schema["description"],
		"message-level description should come from the message's leading comment")
}

func TestMessageInputSchema_SchemaTestMessage(t *testing.T) {
	msg := loadMessage(t, "SchemaTestMessage")
	schema := MessageInputSchema(msg)

	assert.Equal(t, "object", schema["type"])
	assert.Equal(t, false, schema["additionalProperties"])
	assert.NotContains(t, schema, "required")

	props, ok := schema["properties"].(JSONSchema)
	require.True(t, ok)

	t.Run("repeated scalar", func(t *testing.T) {
		assert.Equal(t, JSONSchema{
			"type":        "array",
			"items":       JSONSchema{"type": "integer"},
			"description": "A repeated scalar field.",
		}, props["numbers"])
	})

	t.Run("map to scalar", func(t *testing.T) {
		assert.Equal(t, JSONSchema{
			"type":                 "object",
			"additionalProperties": JSONSchema{"type": "integer"},
			"description":          "A map from string to an int32.",
		}, props["counts"])
	})

	t.Run("map to message", func(t *testing.T) {
		got := props["namedInners"]
		m, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "object", m["type"])
		assert.Equal(t, "A map from string to a message.", m["description"])
		valueSchema, ok := m["additionalProperties"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "object", valueSchema["type"])
		innerProps, ok := valueSchema["properties"].(JSONSchema)
		require.True(t, ok)
		assert.Contains(t, innerProps, "name")
	})

	t.Run("map to enum", func(t *testing.T) {
		got := props["colorByName"]
		m, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "object", m["type"])
		valueSchema, ok := m["additionalProperties"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "string", valueSchema["type"])
		assert.ElementsMatch(t, []any{"COLOR_UNSPECIFIED", "COLOR_RED", "COLOR_GREEN", "COLOR_BLUE"}, valueSchema["enum"])
	})

	t.Run("nested message", func(t *testing.T) {
		got := props["inner"]
		m, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "object", m["type"])
		innerProps, ok := m["properties"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, JSONSchema{"type": "string", "description": "A string field on the nested message."}, innerProps["name"])
	})

	t.Run("recursive message", func(t *testing.T) {
		// Node is self-referential, so every occurrence (including this
		// top-level "root" property) is a $ref into $defs, and the actual
		// object schema lives only once, under $defs.
		got := props["root"]
		ref, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "#/$defs/schemapb.Node", ref["$ref"])

		defs, ok := schema["$defs"].(JSONSchema)
		require.True(t, ok, "$defs should be present for a recursive message")
		nodeSchema, ok := defs["schemapb.Node"].(JSONSchema)
		require.True(t, ok, "$defs should contain the Node schema")

		assert.Equal(t, "object", nodeSchema["type"])
		nodeProps, ok := nodeSchema["properties"].(JSONSchema)
		require.True(t, ok)
		childrenSchema, ok := nodeProps["children"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "array", childrenSchema["type"])
		itemSchema, ok := childrenSchema["items"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "#/$defs/schemapb.Node", itemSchema["$ref"])
	})

	t.Run("enum", func(t *testing.T) {
		got := props["favoriteColor"]
		m, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "string", m["type"])
		assert.ElementsMatch(t, []any{"COLOR_UNSPECIFIED", "COLOR_RED", "COLOR_GREEN", "COLOR_BLUE"}, m["enum"])
	})

	t.Run("repeated enum", func(t *testing.T) {
		got := props["colors"]
		m, ok := got.(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "array", m["type"])
		itemSchema, ok := m["items"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "string", itemSchema["type"])
	})

	t.Run("proto3 optional field is a normal, non-required property", func(t *testing.T) {
		assert.Equal(t, JSONSchema{"type": "string", "description": "A proto3 optional field: never required."}, props["nickname"])
	})

	t.Run("oneof fields are normal properties with a mention of the oneof", func(t *testing.T) {
		email, ok := props["email"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "string", email["type"])
		assert.Contains(t, email["description"], "contact")

		phone, ok := props["phone"].(JSONSchema)
		require.True(t, ok)
		assert.Equal(t, "string", phone["type"])
		assert.Contains(t, phone["description"], "contact")
	})

	wktCases := []struct {
		field string
		want  JSONSchema
	}{
		{"createdAt", JSONSchema{"type": "string", "format": "date-time"}},
		{"ttl", JSONSchema{"type": "string"}},
		{"nicknameWrapper", JSONSchema{"type": "string"}},
		{"countWrapper", JSONSchema{"type": "integer"}},
		{"flagWrapper", JSONSchema{"type": "boolean"}},
		{"blobWrapper", JSONSchema{"type": "string", "contentEncoding": "base64"}},
		{"metadata", JSONSchema{"type": "object"}},
		{"anyValue", JSONSchema{}},
		{"anyList", JSONSchema{"type": "array"}},
		{"updateMask", JSONSchema{"type": "string"}},
		{"nothing", JSONSchema{"type": "object", "additionalProperties": false}},
		{"scoreWrapper", floatSchema()},
		{"ratioWrapper", floatSchema()},
		{"bigCountWrapper", JSONSchema{"type": []any{"integer", "string"}}},
		{"bigUnsignedWrapper", JSONSchema{"type": []any{"integer", "string"}, "minimum": 0}},
		{"smallUnsignedWrapper", JSONSchema{"type": "integer", "minimum": 0}},
		{"anyPayload", anySchema()},
	}
	for _, c := range wktCases {
		t.Run("well-known type "+c.field, func(t *testing.T) {
			assert.Equal(t, c.want, props[c.field], "schema for field %q", c.field)
		})
	}
}

// resolve turns a JSONSchema into a *jsonschema.Resolved, the form needed to
// validate instances, by round-tripping it through JSON (the same
// marshaling the generated MCP server will perform when it builds a
// mcp.Tool's InputSchema).
func resolve(t *testing.T, schema JSONSchema) *jsonschema.Resolved {
	t.Helper()
	data, err := json.Marshal(schema)
	require.NoError(t, err)

	var s jsonschema.Schema
	require.NoError(t, json.Unmarshal(data, &s))

	resolved, err := s.Resolve(nil)
	require.NoError(t, err)
	return resolved
}

// TestSchemaValidatesEmptyMessage checks that an empty (all-zero-value)
// SchemaTestMessage validates against its own generated schema, per the
// issue's property-test acceptance criterion.
func TestSchemaValidatesEmptyMessage(t *testing.T) {
	msg := loadMessage(t, "SchemaTestMessage")
	schema := MessageInputSchema(msg)
	resolved := resolve(t, schema)

	empty := &schemapb.SchemaTestMessage{}
	assertProtoValidatesAgainstSchema(t, resolved, empty)
}

// TestSchemaValidatesPopulatedMessages generates a number of randomly
// populated SchemaTestMessage values and checks that protojson.Marshal(msg)
// validates against the schema generated for SchemaTestMessage, per the
// issue's property-test acceptance criterion.
func TestSchemaValidatesPopulatedMessages(t *testing.T) {
	msg := loadMessage(t, "SchemaTestMessage")
	schema := MessageInputSchema(msg)
	resolved := resolve(t, schema)

	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		m := randomSchemaTestMessage(rnd, 0)
		assertProtoValidatesAgainstSchema(t, resolved, m)
	}
}

// TestAnySchemaAllowsAdditionalProperties locks in that anySchema()
// deliberately leaves "additionalProperties" unset (so it defaults to true):
// unlike every other message schema this builder emits, which forbid
// unknown properties, an Any's "additional" properties are exactly the
// packed message's own fields, which the builder cannot know statically.
// This guards against a future change accidentally tightening anySchema()
// to "additionalProperties": false, which TestSchemaValidatesPopulatedMessages
// alone would not catch, since protojson.Marshal(anypb.New(...)) happens to
// also satisfy a false additionalProperties when the packed message has no
// extra fields colliding with "@type".
func TestAnySchemaAllowsAdditionalProperties(t *testing.T) {
	schema := anySchema()
	assert.NotContains(t, schema, "additionalProperties",
		"anySchema must not set additionalProperties: false, or real Any payloads would be rejected")
}

// TestSchemaValidatesNonFiniteFloats checks that the special NaN/+Inf/-Inf
// string encodings protojson uses for float/double (and the FloatValue/
// DoubleValue wrappers) validate against the generated schema: a plain
// "number" cannot represent them, so the schema must also accept the
// strings protojson actually emits for them.
func TestSchemaValidatesNonFiniteFloats(t *testing.T) {
	msg := loadMessage(t, "SchemaTestMessage")
	schema := MessageInputSchema(msg)
	resolved := resolve(t, schema)

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		m := &schemapb.SchemaTestMessage{
			ScoreWrapper: wrapperspb.Double(v),
			RatioWrapper: wrapperspb.Float(float32(v)),
		}
		assertProtoValidatesAgainstSchema(t, resolved, m)
	}
}

// TestMessageInputSchema_RecursiveRootMessage checks that building a schema
// for a message that is itself part of a reference cycle (rather than one
// reached through a field, as in TestMessageInputSchema_SchemaTestMessage)
// still produces a root schema with "type": "object": the official Go MCP
// SDK requires every tool's input schema to have an object type and panics
// otherwise, so the root may never be a bare {"$ref": ...}.
func TestMessageInputSchema_RecursiveRootMessage(t *testing.T) {
	msg := loadMessage(t, "Node")
	schema := MessageInputSchema(msg)

	require.Equal(t, "object", schema["type"], "root schema must have type \"object\", not a $ref")
	require.NotContains(t, schema, "$ref", "root schema must not itself be a $ref")

	props, ok := schema["properties"].(JSONSchema)
	require.True(t, ok)
	childrenSchema, ok := props["children"].(JSONSchema)
	require.True(t, ok)
	assert.Equal(t, "array", childrenSchema["type"])
	itemSchema, ok := childrenSchema["items"].(JSONSchema)
	require.True(t, ok)
	assert.Equal(t, "#/$defs/schemapb.Node", itemSchema["$ref"])

	defs, ok := schema["$defs"].(JSONSchema)
	require.True(t, ok, "$defs should be present for a recursive root message")
	nodeSchema, ok := defs["schemapb.Node"].(JSONSchema)
	require.True(t, ok, "$defs should contain the Node schema")
	assert.Equal(t, "object", nodeSchema["type"])

	resolved := resolve(t, schema)
	three := &schemapb.Node{
		Value: "root",
		Children: []*schemapb.Node{
			{Value: "child", Children: []*schemapb.Node{
				{Value: "grandchild"},
			}},
		},
	}
	assertProtoValidatesAgainstSchema(t, resolved, three)
}

func assertProtoValidatesAgainstSchema(t *testing.T, resolved *jsonschema.Resolved, m proto.Message) {
	t.Helper()
	data, err := protojson.Marshal(m)
	require.NoError(t, err)

	var instance any
	require.NoError(t, json.Unmarshal(data, &instance))

	if err := resolved.Validate(instance); err != nil {
		t.Fatalf("schema validation failed for %s: %v\ninstance: %s", m.ProtoReflect().Descriptor().FullName(), err, data)
	}
}

func randomSchemaTestMessage(r *rand.Rand, depth int) *schemapb.SchemaTestMessage {
	m := &schemapb.SchemaTestMessage{
		Name:          randomString(r),
		FavoriteColor: randomColor(r),
	}
	for i := 0; i < r.Intn(4); i++ {
		m.Numbers = append(m.Numbers, r.Int31())
	}
	if r.Intn(2) == 0 {
		m.Counts = map[string]int32{"a": r.Int31(), "b": r.Int31()}
	}
	if depth < 2 && r.Intn(2) == 0 {
		m.NamedInners = map[string]*schemapb.Inner{"x": {Name: randomString(r)}}
	}
	if r.Intn(2) == 0 {
		m.ColorByName = map[string]schemapb.Color{"favorite": randomColor(r)}
	}
	if depth < 2 && r.Intn(2) == 0 {
		m.Inner = &schemapb.Inner{Name: randomString(r)}
	}
	if depth < 2 {
		m.Root = randomNode(r, depth+1, 2)
	}
	for i := 0; i < r.Intn(3); i++ {
		m.Colors = append(m.Colors, randomColor(r))
	}
	if r.Intn(2) == 0 {
		nick := randomString(r)
		m.Nickname = &nick
	}
	// The oneof is left unset about a third of the time, since that is a
	// valid (and common) state that the schema must also accept.
	switch r.Intn(3) {
	case 0:
		m.Contact = &schemapb.SchemaTestMessage_Email{Email: randomString(r)}
	case 1:
		m.Contact = &schemapb.SchemaTestMessage_Phone{Phone: randomString(r)}
	case 2:
		// leave Contact nil
	}

	// Well-known types: populate them (instead of leaving them nil) on about
	// half of the generated messages, so the property test also exercises
	// their special-cased schemas, not just the zero-value/absent case.
	if r.Intn(2) == 0 {
		m.CreatedAt = timestamppb.New(timestamppb.Now().AsTime())
		m.Ttl = durationpb.New(time.Duration(r.Intn(1000)) * time.Millisecond)
		m.NicknameWrapper = wrapperspb.String(randomString(r))
		m.CountWrapper = wrapperspb.Int32(r.Int31())
		m.FlagWrapper = wrapperspb.Bool(r.Intn(2) == 0)
		m.BlobWrapper = wrapperspb.Bytes([]byte(randomString(r)))
		st, err := structpb.NewStruct(map[string]any{"k": randomString(r)})
		if err != nil {
			panic(err)
		}
		m.Metadata = st
		v, err := structpb.NewValue(randomString(r))
		if err != nil {
			panic(err)
		}
		m.AnyValue = v
		lv, err := structpb.NewList([]any{randomString(r), r.Int31()})
		if err != nil {
			panic(err)
		}
		m.AnyList = lv
		m.UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{"name", "root.value"}}
		m.ScoreWrapper = wrapperspb.Double(r.Float64())
		m.RatioWrapper = wrapperspb.Float(float32(r.Float64()))
		m.BigCountWrapper = wrapperspb.Int64(r.Int63())
		m.BigUnsignedWrapper = wrapperspb.UInt64(uint64(r.Int63()))
		m.SmallUnsignedWrapper = wrapperspb.UInt32(r.Uint32())
		any, err := anypb.New(&schemapb.Inner{Name: randomString(r)})
		if err != nil {
			panic(err)
		}
		m.AnyPayload = any
	}
	return m
}

func randomNode(r *rand.Rand, depth, maxDepth int) *schemapb.Node {
	n := &schemapb.Node{Value: randomString(r)}
	if depth < maxDepth {
		for i := 0; i < r.Intn(3); i++ {
			n.Children = append(n.Children, randomNode(r, depth+1, maxDepth))
		}
	}
	return n
}

func randomColor(r *rand.Rand) schemapb.Color {
	return schemapb.Color(r.Intn(4))
}

func randomString(r *rand.Rand) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFG"
	n := r.Intn(8)
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}
