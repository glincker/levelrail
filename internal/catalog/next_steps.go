package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed next_steps.json
var nextStepsJSON []byte

// NextStep is one after-install action. Key is a stable code the dashboard
// maps to translated copy; Path is an optional admin path on the app's own URL.
type NextStep struct {
	Key  string `json:"key"`
	Path string `json:"path,omitempty"`
}

var nextStepsByID = func() map[string][]NextStep {
	var m map[string][]NextStep
	if err := json.Unmarshal(nextStepsJSON, &m); err != nil {
		panic("catalog: next_steps.json: " + err.Error())
	}
	return m
}()

// NextSteps returns the after-install checklist for a template, or nil when none is recorded.
func NextSteps(id string) []NextStep { return nextStepsByID[id] }
