//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	// Import the actual config package
	"github.com/davidcollom/awesomegen/internal/config"
)

func main() {
	reflector := config.SchemaReflector()

	// Generate schema for Config struct
	schema := reflector.Reflect(&config.Config{})
	schema.Version = "https://json-schema.org/draft-07/schema"
	// Deliberately no timestamp: the generated file must be byte-for-byte
	// reproducible so CI can detect drift.
	schema.Title = "awesomegen configuration schema"
	schema.Description = "Configuration schema for awesomegen"
	schema.Comments = "This schema is auto-generated from the Go struct definitions. Do not edit directly."
	schema.ID = "https://github.com/davidcollom/awesomegen/blob/main/config-schema.json"

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
	jsonData = append(jsonData, '\n')

	// Write to file
	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		log.Fatalf("Failed to write schema file: %v", err)
	}

	fmt.Printf("Generated JSON schema: %s\n", outputPath)
}
