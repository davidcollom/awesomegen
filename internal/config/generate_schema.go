//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"

	// Import the actual config package
	"github.com/davidcollom/awesomegen/internal/config"
)

func main() {
	// Create a new schema reflector
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            false,
	}

	// Generate schema for Config struct
	schema := reflector.Reflect(&config.Config{})
	schema.Title = "Config"
	schema.Description = "Configuration schema for awesomegen"

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
