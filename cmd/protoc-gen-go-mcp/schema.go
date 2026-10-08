package main

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Full names of the well-known proto message types that get special JSON
// Schema treatment because protojson encodes/decodes them differently from
// an ordinary message with the same fields.
const (
	wktTimestamp   protoreflect.FullName = "google.protobuf.Timestamp"
	wktDuration    protoreflect.FullName = "google.protobuf.Duration"
	wktStruct      protoreflect.FullName = "google.protobuf.Struct"
	wktValue       protoreflect.FullName = "google.protobuf.Value"
	wktListValue   protoreflect.FullName = "google.protobuf.ListValue"
	wktFieldMask   protoreflect.FullName = "google.protobuf.FieldMask"
	wktEmpty       protoreflect.FullName = "google.protobuf.Empty"
	wktDoubleValue protoreflect.FullName = "google.protobuf.DoubleValue"
	wktFloatValue  protoreflect.FullName = "google.protobuf.FloatValue"
	wktInt64Value  protoreflect.FullName = "google.protobuf.Int64Value"
	wktUInt64Value protoreflect.FullName = "google.protobuf.UInt64Value"
	wktInt32Value  protoreflect.FullName = "google.protobuf.Int32Value"
	wktUInt32Value protoreflect.FullName = "google.protobuf.UInt32Value"
	wktBoolValue   protoreflect.FullName = "google.protobuf.BoolValue"
	wktStringValue protoreflect.FullName = "google.protobuf.StringValue"
	wktBytesValue  protoreflect.FullName = "google.protobuf.BytesValue"
	wktAny         protoreflect.FullName = "google.protobuf.Any"
)

// JSONSchema is a JSON Schema (draft 2020-12) object, represented as
// map[string]any so it round-trips through encoding/json (and, via that,
// through github.com/google/jsonschema-go's Schema type).
type JSONSchema map[string]any

// schemaBuilder turns protogen.Message descriptors into JSON Schemas that
// describe exactly what protojson.Unmarshal accepts for that message. It
// detects messages that are part of a reference cycle and represents them
// with "$defs" + "$ref" instead of infinitely inlining them; other message
// types are inlined at every occurrence, even when referenced more than
// once.
type schemaBuilder struct {
	// defs holds the schema for every message that is either recursive or
	// referenced more than once, keyed by the message's proto full name.
	// Entries are populated lazily as messages are visited.
	defs map[protoreflect.FullName]JSONSchema
	// building tracks messages currently being built, to detect recursion.
	building map[protoreflect.FullName]bool
	// refs tracks which message full names ended up needing a $ref (i.e.
	// were part of a reference cycle), so the final schema knows which
	// $defs entries to keep.
	refs map[protoreflect.FullName]bool
}

// MessageInputSchema returns the JSON Schema (draft 2020-12) describing the
// top-level fields of msg, suitable as an MCP tool's inputSchema. The
// schema describes exactly what protojson.Unmarshal accepts for msg: see
// fieldSchema and messageSchema for the field-by-field and
// well-known-type rules.
//
// Design decisions, deliberately stricter than everything protojson.Unmarshal
// actually accepts, to keep the schema clear for LLM clients (which generate
// arguments from it, they don't need to additionally accept every input
// protojson happens to tolerate for backward compatibility):
//
//   - Enum fields only accept the value's name as a string (e.g. "COLOR_RED"),
//     not the underlying int32 number, even though protojson.Unmarshal accepts
//     both.
//   - No field is ever typed to accept JSON null, even though
//     protojson.Unmarshal treats an explicit null the same as the field being
//     absent for most (not all - see wktValue) field types.
//   - Property names are the field's lowerCamel JSON name
//     (field.Desc.JSONName(), what protojson.Marshal emits and the first
//     thing protojson.Unmarshal tries), never the field's original
//     snake_case proto name, even though protojson.Unmarshal also accepts
//     that.
//   - uint32/int32/sint32/fixed32/sfixed32 fields only accept a JSON number,
//     never the decimal string protojson.Unmarshal also accepts for them (as
//     it does for every integer field, not just the 64-bit ones that
//     protojson.Marshal itself emits as strings).
//
// format and contentEncoding keywords on string schemas (e.g. "date-time",
// "base64") are descriptive labels only: github.com/google/jsonschema-go,
// like most JSON Schema implementations, does not validate against them.
func MessageInputSchema(msg *protogen.Message) JSONSchema {
	b := &schemaBuilder{
		defs:     map[protoreflect.FullName]JSONSchema{},
		building: map[protoreflect.FullName]bool{},
		refs:     map[protoreflect.FullName]bool{},
	}
	root := b.messageSchema(msg)
	if len(b.refs) == 0 {
		return root
	}

	// If msg is itself part of a reference cycle, messageSchema returned a
	// bare {"$ref": ...} for it instead of an object schema (so that other
	// occurrences of msg can point at a single $defs entry). An MCP tool's
	// input schema must itself have "type": "object" (the official Go SDK
	// panics in mcp.AddTool otherwise), so unwrap that one level here: use a
	// shallow copy of msg's own $defs entry as the root, instead of a $ref
	// to it. A copy (rather than the $defs entry itself) is required: the
	// root and the $defs entry both need a (different) "$defs" key below,
	// and reusing the same map for both would make it self-referential,
	// which later hangs json.Marshal.
	if root["$ref"] != nil {
		root = JSONSchema{}
		for k, v := range b.defs[msg.Desc.FullName()] {
			root[k] = v
		}
	}

	defs := JSONSchema{}
	for name := range b.refs {
		if s, ok := b.defs[name]; ok {
			defs[string(name)] = s
		}
	}
	root["$defs"] = defs
	return root
}

// messageSchema returns the schema for msg itself: an object schema with one
// property per top-level field, no "required" (proto3 has no required
// fields), and additionalProperties: false.
//
// Well-known types that protojson encodes specially (Timestamp, Duration,
// wrappers, Struct, Value, ListValue, FieldMask, Empty) are special-cased
// here before falling back to the generic message-with-fields handling,
// because protojson does not represent them as a JSON object keyed by field
// name.
func (b *schemaBuilder) messageSchema(msg *protogen.Message) JSONSchema {
	if s := wellKnownTypeSchema(msg.Desc.FullName()); s != nil {
		return s
	}

	fullName := msg.Desc.FullName()
	if b.building[fullName] {
		// Recursion: refer to the (still being built) $defs entry.
		b.refs[fullName] = true
		return refSchema(fullName)
	}
	b.building[fullName] = true
	defer delete(b.building, fullName)

	// Synthetic oneofs exist only to implement proto3 "optional"; their
	// field is still emitted as a normal (non-required) property below, so
	// no special handling is needed here beyond skipping the "part of
	// oneof" note for them (done in fieldSchemaWithDescription).
	properties := JSONSchema{}
	for _, field := range msg.Fields {
		properties[field.Desc.JSONName()] = b.fieldSchemaWithDescription(field)
	}

	schema := JSONSchema{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if desc := fieldDescriptionFromComments(msg.Comments.Leading); desc != "" {
		schema["description"] = desc
	}

	// Remember this message's schema under $defs regardless of whether it
	// turns out to be recursive, so a later recursive reference (reached
	// through some other field) can still find it; MessageInputSchema only
	// keeps the entries actually marked in b.refs in the final output.
	b.defs[fullName] = schema

	// If this message turned out to be recursive, the first (and every)
	// occurrence should point at the $defs entry instead of inlining it.
	if b.refs[fullName] {
		return refSchema(fullName)
	}
	return schema
}

// fieldSchemaWithDescription builds the schema for a single field and adds
// a "description" from its leading proto comment (or the oneof it belongs
// to, if the field has no comment of its own), per the issue's requirement
// that oneof membership be mentioned in the description.
func (b *schemaBuilder) fieldSchemaWithDescription(field *protogen.Field) JSONSchema {
	schema := b.fieldSchema(field)

	desc := fieldDescriptionFromComments(field.Comments.Leading)
	if field.Oneof != nil && !field.Oneof.Desc.IsSynthetic() {
		note := fmt.Sprintf("Part of the %q oneof: at most one of its fields may be set.", field.Oneof.Desc.Name())
		if desc == "" {
			desc = note
		} else {
			desc = desc + " " + note
		}
	}
	if desc != "" {
		schema["description"] = desc
	}
	return schema
}

// fieldSchema returns the schema for a single field's value, handling
// repeated fields, maps, enums, messages, and scalars per the table in the
// issue.
func (b *schemaBuilder) fieldSchema(field *protogen.Field) JSONSchema {
	desc := field.Desc

	if desc.IsMap() {
		// field.Message is the synthetic map-entry message; its second field
		// ("value", field number 2 by the map-entry wire-format convention)
		// is the protogen.Field that actually carries the map's value type,
		// including, for enum or message values, the Enum/Message links that
		// protoreflect.FieldDescriptor.MapValue alone does not provide.
		valueField := mapValueField(field)
		return JSONSchema{
			"type":                 "object",
			"additionalProperties": b.kindSchema(valueField),
		}
	}

	if desc.IsList() {
		return JSONSchema{
			"type":  "array",
			"items": b.kindSchema(field),
		}
	}

	return b.kindSchema(field)
}

// mapValueField returns the protogen.Field describing the value type of a
// map field (field.Message.Fields[1], the "value" field of the synthetic
// map-entry message, conventionally field number 2).
func mapValueField(field *protogen.Field) *protogen.Field {
	for _, f := range field.Message.Fields {
		if f.Desc.Number() == 2 {
			return f
		}
	}
	// Unreachable for a well-formed map field: every map-entry message has
	// exactly a "key" (1) and "value" (2) field.
	panic(fmt.Sprintf("map field %q has no value (number 2) field", field.Desc.FullName()))
}

// kindSchema returns the schema for a single (non-repeated, non-map) value
// described by field: field itself for an ordinary or repeated field, or
// the value field obtained from mapValueField for a map field.
func (b *schemaBuilder) kindSchema(field *protogen.Field) JSONSchema {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return JSONSchema{"type": "boolean"}
	case protoreflect.StringKind:
		return JSONSchema{"type": "string"}
	case protoreflect.BytesKind:
		return JSONSchema{"type": "string", "contentEncoding": "base64"}
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return JSONSchema{"type": "integer"}
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return JSONSchema{"type": "integer", "minimum": 0}
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return JSONSchema{"type": []any{"integer", "string"}}
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return JSONSchema{"type": []any{"integer", "string"}, "minimum": 0}
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return floatSchema()
	case protoreflect.EnumKind:
		return b.enumSchema(field.Enum)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return b.messageSchema(field.Message)
	default:
		// Should be unreachable for a valid FieldDescriptor.
		return JSONSchema{}
	}
}

// enumSchema returns the schema for an enum: a string restricted to the
// enum's value names, since protojson accepts (and emits) enum values as
// their name.
func (b *schemaBuilder) enumSchema(enum *protogen.Enum) JSONSchema {
	values := make([]any, 0, len(enum.Values))
	for _, v := range enum.Values {
		values = append(values, string(v.Desc.Name()))
	}
	schema := JSONSchema{
		"type": "string",
		"enum": values,
	}
	if desc := fieldDescriptionFromComments(enum.Comments.Leading); desc != "" {
		schema["description"] = desc
	}
	return schema
}

// wellKnownTypeSchema returns the special-cased schema for one of the
// well-known proto message types that protojson encodes/decodes as
// something other than a plain JSON object of field names, or nil if
// fullName is not such a type.
func wellKnownTypeSchema(fullName protoreflect.FullName) JSONSchema {
	switch fullName {
	case wktTimestamp:
		return JSONSchema{"type": "string", "format": "date-time"}
	case wktDuration:
		return JSONSchema{"type": "string"}
	case wktStruct:
		return JSONSchema{"type": "object"}
	case wktValue:
		return JSONSchema{}
	case wktListValue:
		return JSONSchema{"type": "array"}
	case wktFieldMask:
		return JSONSchema{"type": "string"}
	case wktEmpty:
		return JSONSchema{"type": "object", "additionalProperties": false}
	case wktAny:
		return anySchema()
	case wktDoubleValue, wktFloatValue:
		return floatSchema()
	case wktInt64Value:
		return JSONSchema{"type": []any{"integer", "string"}}
	case wktUInt64Value:
		return JSONSchema{"type": []any{"integer", "string"}, "minimum": 0}
	case wktInt32Value:
		return JSONSchema{"type": "integer"}
	case wktUInt32Value:
		return JSONSchema{"type": "integer", "minimum": 0}
	case wktBoolValue:
		return JSONSchema{"type": "boolean"}
	case wktStringValue:
		return JSONSchema{"type": "string"}
	case wktBytesValue:
		return JSONSchema{"type": "string", "contentEncoding": "base64"}
	default:
		return nil
	}
}

// refSchema returns a {"$ref": "#/$defs/<name>"} schema pointing at the
// $defs entry for the message with the given full name.
func refSchema(fullName protoreflect.FullName) JSONSchema {
	return JSONSchema{"$ref": "#/$defs/" + string(fullName)}
}

// floatSchema returns the schema for a proto float, double, FloatValue or
// DoubleValue field. protojson marshals NaN and +/-Infinity as the JSON
// strings "NaN", "Infinity" and "-Infinity" (a plain "number" cannot
// represent them), so the schema must accept both a JSON number and one of
// those three strings to describe exactly what protojson produces/accepts.
func floatSchema() JSONSchema {
	return JSONSchema{
		"anyOf": []any{
			JSONSchema{"type": "number"},
			JSONSchema{"type": "string", "enum": []any{"NaN", "Infinity", "-Infinity"}},
		},
	}
}

// anySchema returns the schema for google.protobuf.Any. protojson encodes
// an Any as its unpacked JSON representation plus an injected "@type"
// string field naming the packed message's type; the schema builder has no
// way to know which message types a given Any field may hold (that is a
// runtime property of the registry, not the static proto definition), so
// it only requires the "@type" field protojson always adds and otherwise
// allows arbitrary additional properties for the unpacked message's fields.
// Unlike every other message schema this builder emits, "additionalProperties"
// is deliberately left unset (so it defaults to true) rather than false: the
// unpacked message's fields are exactly those arbitrary additional
// properties, so forbidding them would make the schema reject every real
// Any payload.
func anySchema() JSONSchema {
	return JSONSchema{
		"type": "object",
		"properties": JSONSchema{
			"@type": JSONSchema{"type": "string"},
		},
		"required": []any{"@type"},
	}
}

// fieldDescriptionFromComments turns a leading proto comment into a
// one-line description suitable for a JSON Schema "description", or ""
// if there is no comment.
func fieldDescriptionFromComments(comments protogen.Comments) string {
	if comments == "" {
		return ""
	}
	return processCommentToString(comments)
}
