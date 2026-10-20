package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintPreviewHuman_ShowsModeAndSource(t *testing.T) {
	tests := []struct {
		name   string
		status apiclient.PreviewStatus
		want   []string
	}{
		{
			name:   "metadata with a site image",
			status: apiclient.PreviewStatus{App: "web", Enabled: true, Mode: "metadata", Path: "/", ServerEnabled: true, Latest: &apiclient.PreviewRecord{DeploymentID: "d1", Status: "ok", Source: "og_image"}},
			want:   []string{"previews:   metadata (path /", "ok, source og_image (d1)"},
		},
		{
			name:   "screenshot",
			status: apiclient.PreviewStatus{App: "web", Enabled: true, Mode: "screenshot", Path: "/", ServerEnabled: true, Latest: &apiclient.PreviewRecord{DeploymentID: "d2", Status: "ok", Source: "screenshot"}},
			want:   []string{"previews:   screenshot", "source screenshot"},
		},
		{
			name:   "card",
			status: apiclient.PreviewStatus{App: "web", Enabled: true, Mode: "metadata", Path: "/", ServerEnabled: true, Latest: &apiclient.PreviewRecord{DeploymentID: "d3", Status: "ok", Source: "card"}},
			want:   []string{"source card"},
		},
		{
			name:   "old server without a mode",
			status: apiclient.PreviewStatus{App: "web", Enabled: false, Path: "/", ServerEnabled: true},
			want:   []string{"previews:   off"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printPreviewHuman(&buf, tc.status)
			for _, w := range tc.want {
				if !strings.Contains(buf.String(), w) {
					t.Errorf("output missing %q: %s", w, buf.String())
				}
			}
		})
	}
}
