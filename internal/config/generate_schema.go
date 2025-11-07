//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/stoewer/go-strcase"

	// Import the actual config package
	"github.com/davidcollom/awesomegen/internal/config"
)

func main() {
	// Create a new schema reflector
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            false,
		ExpandedStruct:            true,
		KeyNamer:                  strcase.SnakeCase,
	}
	// err := reflector.AddGoComments("github.com/davidcollom/awesomegen", ".")
	// if err != nil {
	// 	log.Fatalf("Failed to add Go comments: %v", err)
	// }

	// Generate schema for Config struct
	schema := reflector.Reflect(&config.Config{})
	schema.Version = "https://json-schema.org/draft-07/schema"
	schema.Title = fmt.Sprintf("Config.YAML (Generated: %s)", time.Now().Format("2006-01-02 15:04:05"))
	schema.Description = "Configuration schema for awesomegen"
	schema.Comments = "This schema is auto-generated from the Go struct definitions. Do not edit directly."
	schema.ID = "https://example.com/schemas/awesomegen/config-schema.json"

	// Get the output file path (relative to the package directory)
	outputPath := "../../config-schema.json"
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	// Marshal to pretty JSON
	jsonData, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal schema: %v", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		log.Fatalf("Failed to write schema file: %v", err)
	}

	fmt.Printf("Generated JSON schema: %s\n", outputPath)
}
