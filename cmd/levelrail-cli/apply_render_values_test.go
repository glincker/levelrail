package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestRenderPlanValues(t *testing.T) {
	var buf bytes.Buffer
	fields := []iac.FieldChange{
		{Path: "image", Op: iac.OpChange, Old: "app:1", New: "app:2"},
		{Path: "env.A", Op: iac.OpChange, Old: "(hidden)", New: "(hidden)"},
		{Path: "domain", Op: iac.OpAdd, New: "a.example.com"},
	}
	printPlan(&buf, iac.Plan{Changes: []iac.Change{{Action: iac.ActionUpdate, Fields: fields}}})
	out := buf.String()
	for _, want := range []string{"~ image: app:1 -> app:2", "~ env.A: (hidden) -> (hidden)", "+ domain: a.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
