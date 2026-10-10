package platformimport

import "testing"

func TestEngineFromImage_Pgvector(t *testing.T) {
	tests := []struct {
		image, engine, version string
	}{
		{"pgvector/pgvector:pg17", "postgres", "17-pgvector"},
		{"pgvector/pgvector:pg16-bookworm", "postgres", "16-pgvector"},
		{"pgvector/pgvector:0.8.0-pg17", "postgres", "17-pgvector"},
		{"docker.io/pgvector/pgvector:pg18", "postgres", "18-pgvector"},
		{"pgvector/pgvector:pg12", "postgres", ""},
		{"pgvector/pgvector:latest", "postgres", ""},
		{"postgres:16", "postgres", "16"},
		{"postgres:16-alpine", "postgres", "16"},
		{"postgis/postgis:17-3.5", "postgres", "17"},
		{"redis:7", "redis", "7"},
	}
	for _, tc := range tests {
		t.Run(tc.image, func(t *testing.T) {
			engine, version := engineFromImage(tc.image)
			if engine != tc.engine || version != tc.version {
				t.Fatalf("engineFromImage(%q) = %q, %q; want %q, %q", tc.image, engine, version, tc.engine, tc.version)
			}
		})
	}
}
