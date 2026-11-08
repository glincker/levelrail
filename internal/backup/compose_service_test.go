package backup

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestComposeServiceEngineAndDumpCmd(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                    store.EnginePostgres,
		"docker.io/library/postgres":     store.EnginePostgres,
		"postgis/postgis:16-3.4":         store.EnginePostgres,
		"registry.local:5000/mysql:8.0":  store.EngineMySQL,
		"mariadb@sha256:abcd":            store.EngineMariaDB,
		"redis:7":                        "",
		"myorg/postgres-exporter:latest": "",
	}
	for img, want := range cases {
		got, ok := ComposeServiceEngine(img)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q,%v want %q", img, got, ok, want)
		}
	}
	for _, e := range []string{store.EnginePostgres, store.EngineMySQL, store.EngineMariaDB} {
		cmd, err := ComposeDumpCmd(e)
		if err != nil || len(cmd) != 3 || !strings.Contains(cmd[2], "exec ") {
			t.Fatalf("%s: cmd %v err %v", e, cmd, err)
		}
		if strings.Contains(cmd[2], "-p") && e != store.EnginePostgres {
			t.Errorf("%s: password must not be a flag", e)
		}
	}
	if _, err := ComposeDumpCmd(store.EngineRedis); err == nil {
		t.Fatal("redis must be unsupported")
	}
}
