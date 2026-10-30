//go:build jsonschema

package config

import (
	"github.com/invopop/jsonschema"
	"github.com/stoewer/go-strcase"
)

// This file is only compiled with the "jsonschema" build tag, which is used
// by the schema generator (see generate_schema.go). Keeping it behind a tag
// means github.com/invopop/jsonschema - and the release-candidate YAML
// parser it pulls in - are not linked into the production binary.

// SchemaReflector returns a reflector configured for awesomegen's config
// types. It lives behind the "jsonschema" build tag so the schema-only
// dependencies stay out of the released binary.
func SchemaReflector() *jsonschema.Reflector {
	return &jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            false,
		ExpandedStruct:            true,
		KeyNamer:                  strcase.SnakeCase,
	}
}

// JSONSchema provides custom schema for StarsFormat
func (StarsFormat) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{StarsFormatLocale, StarsFormatCompact, StarsFormatNone},
	}
}

// JSONSchema provides custom schema for TopicGroupingMode
func (TopicGroupingMode) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{TopicGroupingModeFlat, TopicGroupingModeNested},
	}
}

// JSONSchema provides custom schema for ItemType
func (ItemType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []interface{}{ItemGitHub, ItemLink},
	}
}
