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
			name:  "valid postgres",
			flags: createDatabaseFlags{name: "main", engine: "postgres", version: "16"},
			want:  databaseResource{Name: "main", Engine: "postgres", Version: "16"},
		},
		{
			name:  "valid redis",
			flags: createDatabaseFlags{name: "cache", engine: "redis", version: "7"},
			want:  databaseResource{Name: "cache", Engine: "redis", Version: "7"},
		},
		{
			name:  "valid mysql",
			flags: createDatabaseFlags{name: "orders", engine: "mysql", version: "8"},
			want:  databaseResource{Name: "orders", Engine: "mysql", Version: "8"},
		},
		{
			name:  "valid mongodb",
			flags: createDatabaseFlags{name: "events", engine: "mongodb", version: "7"},
			want:  databaseResource{Name: "events", Engine: "mongodb", Version: "7"},
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
