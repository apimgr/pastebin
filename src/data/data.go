// Package data embeds static JSON assets compiled into the binary.
// All files in this directory are included via go:embed and accessed through
// the exported helpers below.
package data

import (
	_ "embed"
	"encoding/json"
)

//go:embed languages.json
var languagesJSON []byte

// languagesData is the decoded form of languages.json.
type languagesData struct {
	Languages []string `json:"languages"`
}

// Languages returns all supported Chroma syntax highlighting language
// identifiers, with "text" always first.
func Languages() []string {
	var d languagesData
	if err := json.Unmarshal(languagesJSON, &d); err != nil || len(d.Languages) == 0 {
		return []string{"text"}
	}
	return d.Languages
}
