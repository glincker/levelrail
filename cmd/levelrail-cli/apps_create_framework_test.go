package main

import "testing"

func TestApplyFrameworkPreset(t *testing.T) {
	tests := []struct {
		name      string
		in        createFlags
		wantErr   bool
		wantPort  int
		wantBuild string
	}{
		{name: "none", in: createFlags{}},
		{name: "fills defaults", in: createFlags{framework: "spring-boot"}, wantPort: 8080, wantBuild: "railpack"},
		{name: "keeps explicit port and build type", in: createFlags{framework: "go", port: 9000, buildType: "dockerfile"}, wantPort: 9000, wantBuild: "dockerfile"},
		{name: "unknown", in: createFlags{framework: "cobol"}, wantErr: true},
		{name: "static needs file", in: createFlags{framework: "vite"}, wantErr: true},
		{name: "conflicts with image", in: createFlags{framework: "go", image: "x:1"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.in
			_, err := applyFrameworkPreset(&f)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (f.port != tt.wantPort || f.buildType != tt.wantBuild) {
				t.Fatalf("port=%d build=%q, want %d %q", f.port, f.buildType, tt.wantPort, tt.wantBuild)
			}
		})
	}
}
