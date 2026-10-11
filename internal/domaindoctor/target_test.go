package domaindoctor

import (
	"context"
	"strings"
	"testing"
)

func TestCheckAppUsesStandaloneTarget(t *testing.T) {
	tests := []struct {
		name   string
		target TargetFacts
		state  State
		title  string
	}{
		{"reachable", TargetFacts{Display: "http://127.0.0.1:3000", Reachable: true, Detail: "connected"}, StatePass, "is reachable"},
		{"unreachable", TargetFacts{Display: "http://127.0.0.1:3000", Detail: "connection refused"}, StateFail, "cannot reach"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.target
			rn := &run{facts: Facts{Domain: "tools.example.com", App: "_standalone", Target: &target}}
			c := rn.checkApp(context.Background())
			if c.State != tt.state || !strings.Contains(c.Title, tt.title) || c.Detail != tt.target.Detail {
				t.Fatalf("check = %+v, want state %s and title containing %q", c, tt.state, tt.title)
			}
			if c.Fix != nil && c.Fix.Action != nil {
				t.Fatal("a standalone target must not offer the app start action")
			}
		})
	}
}
