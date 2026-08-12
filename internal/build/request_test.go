package build

import (
	"errors"
	"testing"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     Request
		wantErr error
	}{
		{
			name: "valid",
			req:  Request{ContextDir: "testdata", Tag: "levelrail-spike:latest"},
		},
		{
			name:    "missing context dir",
			req:     Request{Tag: "levelrail-spike:latest"},
			wantErr: ErrContextDirRequired,
		},
		{
			name:    "missing tag",
			req:     Request{ContextDir: "testdata"},
			wantErr: ErrTagRequired,
		},
		{
			name:    "missing both reports context dir first",
			req:     Request{},
			wantErr: ErrContextDirRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequest_dockerfilePath(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{
			name: "defaults to ContextDir/Dockerfile",
			req:  Request{ContextDir: "testdata"},
			want: "testdata/Dockerfile",
		},
		{
			name: "explicit path wins",
			req:  Request{ContextDir: "testdata", DockerfilePath: "testdata/Dockerfile.prod"},
			want: "testdata/Dockerfile.prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.dockerfilePath(); got != tt.want {
				t.Fatalf("dockerfilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}
