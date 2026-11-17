package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	const body = `services:
  app:
    image: example/app:1
    environment:
      KEY: $SERVICE_PASSWORD_32_APPKEY
      SECRET: ${SERVICE_HEX_64_Y}
    volumes:
      - app_data:/data
`
	tests := []struct {
		name    string
		body    string
		want    []*regexp.Regexp
		wantErr string
	}{
		{"generated secrets and volume", body, []*regexp.Regexp{
			regexp.MustCompile(`KEY: [A-Za-z0-9]{32}\n`),
			regexp.MustCompile(`SECRET: [0-9a-f]{64}\n`),
			regexp.MustCompile(`(?m)^volumes:\n\s+app_data: \{\}`),
		}, ""},
		{"unknown magic var is rejected", "services:\n  a:\n    image: x\n    environment:\n      K: $SERVICE_NOPE_Z\n", nil, "SERVICE_NOPE_Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := render(tt.body)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, re := range tt.want {
				if !re.MatchString(out) {
					t.Errorf("output missing %v:\n%s", re, out)
				}
			}
		})
	}
}
