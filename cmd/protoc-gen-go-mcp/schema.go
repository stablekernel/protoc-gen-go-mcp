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
)

// JSONSchema is a JSON Schema (draft 2020-12) object, represented as
// map[string]any so it round-trips through encoding/json (and, via that,
// through github.com/google/jsonschema-go's Schema type).
type JSONSchema map[string]any

// schemaBuilder turns protogen.Message descriptors into JSON Schemas that
// describe exactly what protojson.Unmarshal accepts for that message. It
// caches message schemas so recursive and repeated message types are only
// computed once, and represents recursive messages with "$defs" + "$ref"
// instead of infinitely inlining them.
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

	// If this message turned out to be recursive, remember its schema under
	// $defs and have the first reference point back to it too, so every
	// occurrence (including the top-level one, when msg is itself
	// recursive and reached again elsewhere) is consistent.
	if b.refs[fullName] {
		b.defs[fullName] = schema
		return refSchema(fullName)
	}
	b.defs[fullName] = schema
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
		valueSchema := b.kindSchema(field, desc.MapValue())
		return JSONSchema{
			"type":                 "object",
			"additionalProperties": valueSchema,
		}
	}

	if desc.IsList() {
		return JSONSchema{
			"type":  "array",
			"items": b.kindSchema(field, desc),
		}
	}

	return b.kindSchema(field, desc)
}

// kindSchema returns the schema for a single (non-repeated, non-map) value
// of the given field descriptor's kind. field is the protogen.Field that
// fd was derived from (itself, or its map value), used to reach the
// corresponding *protogen.Message/*protogen.Enum for message/enum kinds.
func (b *schemaBuilder) kindSchema(field *protogen.Field, fd protoreflect.FieldDescriptor) JSONSchema {
	switch fd.Kind() {
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
		return JSONSchema{"type": "number"}
	case protoreflect.EnumKind:
		return b.enumSchema(field.Enum)
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return b.messageSchemaForField(field)
	default:
		// Should be unreachable for a valid FieldDescriptor.
		return JSONSchema{}
	}
}

// messageSchemaForField resolves the *protogen.Message that corresponds to
// field's message kind. For a map field, field.Message is the synthetic
// map-entry message, not the value message, so the caller must instead pass
// a field that already points at the value; messageSchemaForField handles
// both the plain-message and map-value cases by preferring the explicit
// value message when the field itself is a map.
func (b *schemaBuilder) messageSchemaForField(field *protogen.Field) JSONSchema {
	msg := field.Message
	if field.Desc.IsMap() {
		// field.Message is the map-entry message; its second field ("value")
		// has the real value message, if any.
		for _, f := range field.Message.Fields {
			if f.Desc.Number() == 2 {
				msg = f.Message
			}
		}
	}
	if msg == nil {
		return JSONSchema{}
	}
	return b.messageSchema(msg)
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
	case wktDoubleValue, wktFloatValue:
		return JSONSchema{"type": "number"}
	case wktInt64Value, wktUInt64Value:
		return JSONSchema{"type": []any{"integer", "string"}}
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

// fieldDescriptionFromComments turns a leading proto comment into a
// one-line description suitable for a JSON Schema "description", or ""
// if there is no comment.
func fieldDescriptionFromComments(comments protogen.Comments) string {
	if comments == "" {
		return ""
	}
	return processCommentToString(comments)
}
