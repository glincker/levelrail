package dbupgrade

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHealthCommand_SearchEngines(t *testing.T) {
	tests := []struct {
		engine, shell, keyRef, path string
	}{
		{store.EngineMeilisearch, "sh", `"$MEILI_MASTER_KEY"`, "/version"},
		{store.EngineQdrant, "bash", `"$QDRANT__SERVICE__API_KEY"`, "get 6333 /"},
		{store.EngineTypesense, "bash", `"$TYPESENSE_API_KEY"`, "/debug"},
		{store.EngineMemcached, "bash", "", "STAT version"},
	}
	for _, tc := range tests {
		t.Run(tc.engine, func(t *testing.T) {
			cmd, err := HealthCommand(tc.engine)
			if err != nil {
				t.Fatal(err)
			}
			if cmd[0] != tc.shell || !strings.Contains(cmd[2], tc.keyRef) || !strings.Contains(cmd[2], tc.path) || !strings.Contains(cmd[2], `echo "$v"`) {
				t.Fatalf("health command = %q", cmd)
			}
		})
	}
}
