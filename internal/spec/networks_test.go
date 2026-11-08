package spec

import (
	"strings"
	"testing"
)

func TestAppNetworks_Parse(t *testing.T) {
	base := "version: 1\nservices:\n  web:\n    build:\n      type: image\n      image: nginx:1\n    port: 80\n"
	tests := []struct {
		name, nets, wantErr string
	}{
		{"valid", "    networks:\n      - name: shared-db\n        aliases: [pg, primary]\n", ""},
		{"duplicate", "    networks:\n      - {name: a}\n      - {name: a}\n", "twice"},
		{"bad alias", "    networks:\n      - {name: a, aliases: [Bad_Alias]}\n", "aliases"},
		{"bad name", "    networks:\n      - {name: Bad}\n", "networks"},
		{"unknown field", "    networks:\n      - {name: a, driver: x}\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse([]byte(base + tt.nets))
			if tt.name == "unknown field" {
				if err == nil {
					t.Fatal("unknown network field must be rejected")
				}
				return
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if got := s.Services["web"].Networks[0].Aliases; len(got) != 2 {
					t.Errorf("aliases = %v", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
