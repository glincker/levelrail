package api

import (
	"encoding/json"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// cloneRuntime deep-copies the options so an edited clone never aliases its source.
func cloneRuntime(r *spec.RuntimeOptions) *spec.RuntimeOptions {
	if r == nil {
		return nil
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	var out spec.RuntimeOptions
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}
