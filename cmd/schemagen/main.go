package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/davidcollom/awesomegen/internal/config"

	"github.com/invopop/jsonschema"
)

// TODO: Make this run during Release process or if config changes.
func main() {
	reflector := jsonschema.Reflector{
		FieldNameTag: "yaml",
	}
	schema := reflector.Reflect(&config.Config{})

	var out *os.File
	if len(os.Args) > 1 {
		var err error
		out, err = os.Create(os.Args[1])
		if err != nil {
			panic(err)
		}
		defer out.Close()
	} else {
		out = os.Stdout
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	enc.Encode(schema)
	// Lets send to STDERR so that stdout can still be used in a pipe.
	fmt.Fprintln(os.Stderr, "Successfully Generated JSON schema!")
}
