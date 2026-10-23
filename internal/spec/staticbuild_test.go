package spec

import "testing"

func TestValidateStaticBuild(t *testing.T) {
	tests := []struct {
		name    string
		build   Build
		wantErr bool
	}{
		{"served as-is", Build{Type: BuildStatic, Path: "dist"}, false},
		{"command and publish", Build{Type: BuildStatic, Command: "npm run build", Publish: "dist"}, false},
		{"nested publish with args", Build{Type: BuildStatic, Command: "npm run build", Publish: "apps/web/dist", Args: map[string]string{"VITE_API": "x"}}, false},
		{"command without publish", Build{Type: BuildStatic, Command: "npm run build"}, true},
		{"publish without command", Build{Type: BuildStatic, Publish: "dist"}, true},
		{"path and command", Build{Type: BuildStatic, Command: "make", Publish: "dist", Path: "dist"}, true},
		{"multi-line command", Build{Type: BuildStatic, Command: "a\nb", Publish: "dist"}, true},
		{"publish escapes", Build{Type: BuildStatic, Command: "make", Publish: "../x"}, true},
		{"publish absolute", Build{Type: BuildStatic, Command: "make", Publish: "/x"}, true},
		{"publish is root", Build{Type: BuildStatic, Command: "make", Publish: "."}, true},
		{"bad arg name", Build{Type: BuildStatic, Command: "make", Publish: "dist", Args: map[string]string{"A B": "x"}}, true},
		{"command on railpack", Build{Type: BuildRailpack, Command: "make"}, true},
		{"publish on dockerfile", Build{Type: BuildDockerfile, Publish: "dist"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.build.validateStaticBuild("web")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
