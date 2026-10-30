// Package awesomegen exposes the artefacts of the awesomegen module that
// are useful outside of the command line interface.
//
// The JSON schema of the configuration file is embedded here so that the
// `awesomegen schema` command can print it without linking the schema
// generator - and the release candidate YAML parser it pulls in - into the
// released binary. The embedded copy is the same file that is checked in at
// the root of the repository, and the continuous integration verifies that
// the file is still what the Go types produce.
package awesomegen

import _ "embed"

// ConfigSchema is the JSON schema of the configuration file.
//
//go:embed config-schema.json
var ConfigSchema []byte
