// Package product exposes build identity generated from build/config.yml.
// Run scripts/sync-build-metadata.py to update the embedded metadata and plist
// together; the normal application build does this before compiling Go.
package product

import (
	_ "embed"
	"encoding/json"
)

//go:embed info.json
var data []byte

type Metadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	BuildNumber string `json:"buildNumber"`
	Identifier  string `json:"identifier"`
	Description string `json:"description"`
}

var metadata = func() Metadata {
	var value Metadata
	if err := json.Unmarshal(data, &value); err != nil {
		panic(err)
	}
	if value.Name == "" || value.Version == "" || value.Identifier == "" {
		panic("incomplete product metadata")
	}
	return value
}()

// Info returns a value copy so callers cannot change process-wide identity.
func Info() Metadata { return metadata }
