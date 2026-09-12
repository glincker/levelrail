package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestPlanDatabaseCreate(t *testing.T) {
	tests := []struct {
		name    string
		flags   createDatabaseFlags
		wantErr string // substring; empty means no error
		want    databaseResource
	}{
		{
			name:    "missing everything",
			flags:   createDatabaseFlags{},
			wantErr: "--name, --engine, --version",
		},
		{
			name:    "missing name",
			flags:   createDatabaseFlags{engine: "postgres", version: "16"},
			wantErr: "--name",
		},
		{
			name:    "missing engine",
			flags:   createDatabaseFlags{name: "main", version: "16"},
			wantErr: "--engine",
		},
		{
			name:    "missing version",
			flags:   createDatabaseFlags{name: "main", engine: "postgres"},
			wantErr: "--version",
		},
		{
			name:    "unsupported engine",
			flags:   createDatabaseFlags{name: "main", engine: "cassandra", version: "5"},
			wantErr: "--engine must be one of",
		},
		{
			name:  "node-id omitted leaves NodeID unset",
			flags: createDatabaseFlags{name: "main", engine: "postgres", version: "16"},
			want:  databaseResource{Name: "main", Engine: "postgres", Version: "16"},
		},
		{
			name:  "node-id explicitly set applies an override",
			flags: createDatabaseFlags{name: "main", engine: "postgres", version: "16", nodeID: "node_a", nodeIDSet: true},
			want:  databaseResource{Name: "main", Engine: "postgres", Version: "16", NodeID: "node_a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planDatabaseCreate(tt.flags)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("planDatabaseCreate() error = nil, want substring %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("planDatabaseCreate() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("planDatabaseCreate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestPlanDatabaseCreate_ValidEngines covers every supported engine's
// happy path: a flat (name, engine, version) tuple list rather than a
// struct-literal-per-case table, since every one of these cases has the
// exact same "name/engine/version in, matching databaseResource out"
// shape and repeating that shape eight times as nested struct literals
// (the form this test used before) is what triggered a duplication
// finding.
func TestPlanDatabaseCreate_ValidEngines(t *testing.T) {
	cases := [][3]string{
		{"main", "postgres", "16"},
		{"cache", "redis", "7"},
		{"orders", "mysql", "8"},
		{"events", "mongodb", "7"},
		{"orders", "mariadb", "11"},
		{"cache", "keydb", "latest"},
		{"hotcache", "dragonfly", "v1.27.1"},
		{"analytics", "clickhouse", "24.8"},
	}

	for _, c := range cases {
		name, engine, version := c[0], c[1], c[2]
		t.Run("valid "+engine, func(t *testing.T) {
			got, err := planDatabaseCreate(createDatabaseFlags{name: name, engine: engine, version: version})
			if err != nil {
				t.Fatalf("planDatabaseCreate() error = %v", err)
			}
			want := databaseResource{Name: name, Engine: engine, Version: version}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("planDatabaseCreate() = %+v, want %+v", got, want)
			}
		})
	}
}
