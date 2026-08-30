package database

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestContainerName(t *testing.T) {
	if got, want := ContainerName("main"), "db-main"; got != want {
		t.Errorf("ContainerName(%q) = %q, want %q", "main", got, want)
	}
}

func TestContainerPort(t *testing.T) {
	tests := []struct {
		engine string
		want   int
		wantOK bool
	}{
		{engine: store.EnginePostgres, want: 5432, wantOK: true},
		{engine: store.EngineRedis, want: 6379, wantOK: true},
		{engine: store.EngineMySQL, want: 3306, wantOK: true},
		{engine: store.EngineMongoDB, want: 27017, wantOK: true},
		{engine: store.EngineMariaDB, want: 3306, wantOK: true},
		{engine: store.EngineKeyDB, want: 6379, wantOK: true},
		{engine: store.EngineDragonfly, want: 6379, wantOK: true},
		{engine: "not-a-real-engine", want: 0, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			got, ok := ContainerPort(tt.engine)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ContainerPort(%q) = (%d, %v), want (%d, %v)", tt.engine, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPasswordSecretKey(t *testing.T) {
	tests := []struct {
		engine string
		want   string
		wantOK bool
	}{
		{engine: store.EnginePostgres, want: PostgresPasswordEnvKey, wantOK: true},
		{engine: store.EngineMySQL, want: MySQLPasswordEnvKey, wantOK: true},
		{engine: store.EngineMongoDB, want: MongoPasswordEnvKey, wantOK: true},
		{engine: store.EngineMariaDB, want: MariaDBPasswordEnvKey, wantOK: true},
		{engine: store.EngineRedis, want: "", wantOK: false},
		{engine: store.EngineKeyDB, want: "", wantOK: false},
		{engine: store.EngineDragonfly, want: "", wantOK: false},
		{engine: "not-a-real-engine", want: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			got, ok := PasswordSecretKey(tt.engine)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("PasswordSecretKey(%q) = (%q, %v), want (%q, %v)", tt.engine, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestSupportsField(t *testing.T) {
	tests := []struct {
		engine string
		field  string
		want   bool
	}{
		{engine: store.EnginePostgres, field: "url", want: true},
		{engine: store.EnginePostgres, field: "host", want: true},
		{engine: store.EnginePostgres, field: "port", want: true},
		{engine: store.EnginePostgres, field: "username", want: true},
		{engine: store.EnginePostgres, field: "password", want: true},
		{engine: store.EnginePostgres, field: "database", want: true},
		{engine: store.EnginePostgres, field: "not-a-real-field", want: false},

		{engine: store.EngineRedis, field: "url", want: true},
		{engine: store.EngineRedis, field: "host", want: true},
		{engine: store.EngineRedis, field: "port", want: true},
		{engine: store.EngineRedis, field: "username", want: false},
		{engine: store.EngineRedis, field: "password", want: false},
		{engine: store.EngineRedis, field: "database", want: false},

		{engine: store.EngineMariaDB, field: "url", want: true},
		{engine: store.EngineMariaDB, field: "username", want: true},
		{engine: store.EngineMariaDB, field: "password", want: true},
		{engine: store.EngineMariaDB, field: "database", want: true},

		{engine: store.EngineKeyDB, field: "url", want: true},
		{engine: store.EngineKeyDB, field: "port", want: true},
		{engine: store.EngineKeyDB, field: "username", want: false},
		{engine: store.EngineKeyDB, field: "password", want: false},
		{engine: store.EngineKeyDB, field: "database", want: false},

		{engine: store.EngineDragonfly, field: "url", want: true},
		{engine: store.EngineDragonfly, field: "host", want: true},
		{engine: store.EngineDragonfly, field: "port", want: true},
		{engine: store.EngineDragonfly, field: "username", want: false},
		{engine: store.EngineDragonfly, field: "password", want: false},
		{engine: store.EngineDragonfly, field: "database", want: false},

		{engine: "not-a-real-engine", field: "url", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.engine+"/"+tt.field, func(t *testing.T) {
			if got := SupportsField(tt.engine, tt.field); got != tt.want {
				t.Errorf("SupportsField(%q, %q) = %v, want %v", tt.engine, tt.field, got, tt.want)
			}
		})
	}
}
