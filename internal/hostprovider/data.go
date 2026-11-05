package hostprovider

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// EnvDataFile names an optional JSON file whose provider entries replace
// the embedded defaults, so links and steps change without a rebuild.
const EnvDataFile = "LEVELRAIL_HOSTPROVIDER_DATA_FILE"

//go:embed hostprovider_data.json
var embeddedData []byte

// Entry is the editable text for one provider.
type Entry struct {
	Name    string   `json:"name"`
	DocsURL string   `json:"docs_url"`
	Steps   []string `json:"steps"`
	Note    string   `json:"note,omitempty"`
}

// Data maps a provider id to its entry.
type Data map[string]Entry

// LoadData returns the embedded defaults merged with the optional override
// file. A broken override keeps the defaults and is reported as the error.
func LoadData() (Data, error) {
	data := Data{}
	if err := json.Unmarshal(embeddedData, &data); err != nil {
		return Data{}, fmt.Errorf("hostprovider data: embedded: %w", err)
	}
	path := strings.TrimSpace(os.Getenv(EnvDataFile))
	if path == "" {
		return data, nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-chosen override path
	if err != nil {
		return data, fmt.Errorf("hostprovider data: %w", err)
	}
	over := Data{}
	if err := json.Unmarshal(raw, &over); err != nil {
		return data, fmt.Errorf("hostprovider data: parse %s: %w", path, err)
	}
	for id, o := range over {
		e := data[id]
		if o.Name != "" {
			e.Name = o.Name
		}
		if o.DocsURL != "" {
			e.DocsURL = o.DocsURL
		}
		if o.Steps != nil {
			e.Steps = o.Steps
		}
		if o.Note != "" {
			e.Note = o.Note
		}
		data[id] = e
	}
	return data, nil
}
