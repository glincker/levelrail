package spec

import "testing"

func TestNormalizeRepoPath(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"empty", "", "", false},
		{"dot", ".", "", false},
		{"dot slash", "./apps/web/", "apps/web", false},
		{"clean", "apps//web/../api", "apps/api", false},
		{"spaces", "  deploy/Dockerfile ", "deploy/Dockerfile", false},
		{"absolute", "/etc/passwd", "", true},
		{"windows drive", "C:/x", "", true},
		{"backslash", `apps\web`, "", true},
		{"parent", "..", "", true},
		{"escape", "apps/../../x", "", true},
		{"nul", "a\x00b", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRepoPath("path", tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveGitBuild(t *testing.T) {
	cases := []struct {
		name, typ, base, path string
		want                  Build
		wantErr               bool
	}{
		{"root dockerfile", BuildDockerfile, "", "Dockerfile", Build{Type: BuildDockerfile, Path: "Dockerfile"}, false},
		{"turbo prune root context", BuildDockerfile, "", "apps/glinr/deploy/Dockerfile", Build{Type: BuildDockerfile, Path: "apps/glinr/deploy/Dockerfile"}, false},
		{"base dir with path inside", BuildDockerfile, "apps/api", "apps/api/Dockerfile.prod", Build{Type: BuildDockerfile, BaseDirectory: "apps/api", Path: "Dockerfile.prod"}, false},
		{"base dir, default dockerfile", BuildDockerfile, "apps/api", "", Build{Type: BuildDockerfile, BaseDirectory: "apps/api"}, false},
		{"path outside base", BuildDockerfile, "apps/api", "Dockerfile", Build{}, true},
		{"sibling prefix is not inside", BuildDockerfile, "apps/api", "apps/api2/Dockerfile", Build{}, true},
		{"railpack with base", BuildRailpack, "apps/web", "", Build{Type: BuildRailpack, BaseDirectory: "apps/web"}, false},
		{"railpack with path", BuildRailpack, "", "Dockerfile", Build{}, true},
		{"escaping base", BuildDockerfile, "../x", "", Build{}, true},
		{"absolute path", BuildDockerfile, "", "/Dockerfile", Build{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveGitBuild(tc.typ, tc.base, tc.path)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && (got.Type != tc.want.Type || got.Path != tc.want.Path || got.BaseDirectory != tc.want.BaseDirectory) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
