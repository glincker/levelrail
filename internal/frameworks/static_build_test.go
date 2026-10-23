package frameworks

import "testing"

func TestStaticBuildPrefill(t *testing.T) {
	tests := []struct {
		id          string
		wantPublish string
		wantOK      bool
	}{
		{"vite", "dist", true},
		{"astro", "dist", true},
		{"nextjs-static", "out", true},
		{"docusaurus", "build", true},
		{"nextjs", "", false},
		{"go", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			p, ok := Get(tt.id)
			if !ok {
				t.Fatalf("missing preset %s", tt.id)
			}
			cmd, publish, ok := p.StaticBuild()
			if ok != tt.wantOK || publish != tt.wantPublish {
				t.Fatalf("StaticBuild() = %q, %q, %v", cmd, publish, ok)
			}
			if ok && cmd != DefaultStaticBuildCommand {
				t.Fatalf("command = %q", cmd)
			}
		})
	}
}
