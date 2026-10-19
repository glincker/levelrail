package mcptools

import (
	"encoding/json"
	"testing"
)

// maxToolDescriptionChars is the description cap: one or two sentences.
const maxToolDescriptionChars = 400

func TestToolDescriptionLength(t *testing.T) {
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true}) {
		if n := len(tool.Description); n > maxToolDescriptionChars {
			t.Errorf("%s description is %d chars, max %d: keep one or two sentences", tool.Name, n, maxToolDescriptionChars)
		}
	}
}

// maxRepeatedSchemaFragments is the ratchet on identical long property descriptions repeated across tools.
const maxRepeatedSchemaFragments = 0

func TestNoDuplicateSchemaFragments(t *testing.T) {
	seen := map[string]int{}
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true}) {
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		for _, p := range schema.Properties {
			if len(p.Description) >= 60 {
				seen[p.Description]++
			}
		}
	}
	dups := 0
	for d, n := range seen {
		if n > 2 {
			dups++
			t.Logf("property description repeated %d times: %.80s", n, d)
		}
	}
	if dups > maxRepeatedSchemaFragments {
		t.Errorf("%d long property descriptions repeat across more than two tools, over the %d ratchet: share one type or shorten them", dups, maxRepeatedSchemaFragments)
	}
}
