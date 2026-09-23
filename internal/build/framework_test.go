package build

import "testing"

func TestFrameworkLabel(t *testing.T) {
	tests := []struct {
		provider  string
		wantLabel string
		wantOK    bool
	}{
		{provider: "node", wantLabel: "Node.js", wantOK: true},
		{provider: "golang", wantLabel: "Go", wantOK: true},
		{provider: "java", wantLabel: "Java (Spring Boot)", wantOK: true},
		{provider: "python", wantLabel: "", wantOK: false},
		{provider: "", wantLabel: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			label, ok := FrameworkLabel(tt.provider)
			if label != tt.wantLabel || ok != tt.wantOK {
				t.Errorf("FrameworkLabel(%q) = (%q, %v), want (%q, %v)", tt.provider, label, ok, tt.wantLabel, tt.wantOK)
			}
		})
	}
}
