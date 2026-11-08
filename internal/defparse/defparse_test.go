package defparse

import (
	"strings"
	"testing"
)

func TestDefparseParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Def
		wantErr string
	}{
		{name: "path", in: `{"schemaVersion":2,"dockerfilePath":"./docker/Dockerfile"}`, want: Def{DockerfilePath: "docker/Dockerfile"}},
		{name: "image", in: `{"schemaVersion":2,"imageName":"nginx:1"}`, want: Def{ImageName: "nginx:1"}},
		{name: "lines", in: `{"schemaVersion":2,"dockerfileLines":["FROM alpine"]}`, want: Def{DockerfileLines: []string{"FROM alpine"}}},
		{name: "schema 1", in: `{"schemaVersion":1,"templateId":"node/8.7.0"}`, wantErr: "use a Dockerfile"},
		{name: "unknown schema", in: `{"schemaVersion":3,"imageName":"x"}`, wantErr: "not supported"},
		{name: "absolute", in: `{"schemaVersion":2,"dockerfilePath":"/etc/passwd"}`, wantErr: "must be relative"},
		{name: "traversal", in: `{"schemaVersion":2,"dockerfilePath":"a/../../x"}`, wantErr: "inside the project"},
		{name: "empty", in: `{"schemaVersion":2}`, wantErr: "sets none"},
		{name: "not json", in: `nope`, wantErr: "not valid JSON"},
		{name: "oversize", in: `{"schemaVersion":2,"imageName":"` + strings.Repeat("a", MaxBytes) + `"}`, wantErr: "larger than"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse([]byte(tc.in))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.DockerfilePath != tc.want.DockerfilePath || got.ImageName != tc.want.ImageName || len(got.DockerfileLines) != len(tc.want.DockerfileLines) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
