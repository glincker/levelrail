package search

import (
	"slices"
	"testing"
)

func TestHighlightSpans(t *testing.T) {
	tests := []struct {
		name  string
		title string
		terms []string
		want  []Span
	}{
		{"substring", "Restart api-gateway", []string{"api"}, []Span{{8, 11}}},
		{"repeated", "api-api", []string{"api"}, []Span{{0, 3}, {4, 7}}},
		{"overlap merges", "restart", []string{"rest", "start"}, []Span{{0, 7}}},
		{"typo marks shared trigrams", "rapid-worker", []string{"rapd"}, []Span{{0, 3}}},
		{"short term without match", "web", []string{"zz"}, nil},
		{"no terms", "web", nil, nil},
		{"code points not bytes", "café-api", []string{"api"}, []Span{{5, 8}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := highlightSpans(tt.title, tt.terms)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("highlightSpans(%q, %v) = %v, want %v", tt.title, tt.terms, got, tt.want)
			}
		})
	}
}

func TestActionCall(t *testing.T) {
	tool, args, needs := actionCall(Doc{ActionID: "app.deploy", Name: "api"})
	if tool != "deploy_app" || args["name"] != "api" || !slices.Equal(needs, []string{"image"}) {
		t.Fatalf("deploy call = %q %v %v", tool, args, needs)
	}
	if tool, _, _ := actionCall(Doc{ActionID: "app.stop", Name: "api"}); tool != "" {
		t.Fatalf("stop has no MCP tool, got %q", tool)
	}
}
