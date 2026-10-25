package main

import (
	"bytes"
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
			name:    "search engine refuses latest",
			flags:   createDatabaseFlags{name: "search", engine: "meilisearch", version: "latest"},
			wantErr: "pinned release tag",
		},
		{
			name:    "qdrant tag needs its v prefix",
			flags:   createDatabaseFlags{name: "vec", engine: "qdrant", version: "1.19"},
			wantErr: "pinned release tag",
		},
		{
			name:    "memcached refuses latest",
			flags:   createDatabaseFlags{name: "cache", engine: "memcached", version: "latest"},
			wantErr: "pinned release tag",
		},
		{
			name:    "typesense tag has no v prefix",
			flags:   createDatabaseFlags{name: "ts", engine: "typesense", version: "v30.2"},
			wantErr: "pinned release tag",
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

// Table of (name, engine, version) tuples covering every supported engine's happy path.
func TestPlanDatabaseCreate_ValidEngines(t *testing.T) {
	cases := [][3]string{
		{"main", "postgres", "16"},
		{"cache", "redis", "7"},
		{"orders", "mysql", "8"},
		{"events", "mongodb", "7"},
		{"orders", "mariadb", "11"},
		{"cache", "keydb", "latest"},
		{"hotcache", "dragonfly", "v1.27.1"},
		{"cache", "valkey", "8"},
		{"search", "meilisearch", "v1.55"},
		{"vectors", "qdrant", "v1.19.2"},
		{"catalog", "typesense", "30.2"},
		{"sessions", "memcached", "1.6"},
		{"geo", "postgres", "17-postgis"},
		{"metrics", "postgres", "17-timescaledb"},
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

func TestPlanDatabaseCreate_ExistingVolume(t *testing.T) {
	tests := []struct {
		choice  string
		wantErr bool
	}{{"", false}, {"reuse", false}, {"discard", false}, {"maybe", true}}
	for _, tt := range tests {
		plan, err := planDatabaseCreate(createDatabaseFlags{name: "main", engine: "postgres", version: "16", existingVolume: tt.choice})
		if (err != nil) != tt.wantErr {
			t.Fatalf("choice %q: err = %v, wantErr %v", tt.choice, err, tt.wantErr)
		}
		if err == nil && plan.ExistingVolume != tt.choice {
			t.Errorf("choice %q: ExistingVolume = %q", tt.choice, plan.ExistingVolume)
		}
	}
}

func TestRunDatabasesCreate_PositionalNameGetsHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := runDatabasesCreate("levelrail-cli-test", []string{"v2-fresh"}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitValidation || !strings.Contains(stderr.String(), "--name v2-fresh") {
		t.Fatalf("exit = %d stderr = %q, want a --name hint", got, stderr.String())
	}
}
