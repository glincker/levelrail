package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/quota"
)

func TestAppResources_RejectsNegativeAndOversizeShm(t *testing.T) {
	t.Setenv("APP_COMPOSE_MAX_SHM_BYTES", "")
	rt, db, cookie, proj := quotaFixture(t)
	cases := map[string]string{
		"negative memory": `,"resources":{"memory_bytes":-1073741824}`,
		"negative cpu":    `,"resources":{"nano_cpus":-1}`,
		"negative swap":   `,"resources":{"swap_memory_bytes":-1}`,
		"negative shm":    `,"resources":{"shm_size_bytes":-1}`,
		"huge shm":        `,"resources":{"shm_size_bytes":1000000000000000}`,
	}
	for name, extra := range cases {
		code, body := createQuotaApp(t, rt, cookie, "bad", proj, extra)
		var e codedError
		_ = json.Unmarshal([]byte(body), &e)
		if code != http.StatusBadRequest || e.Code != invalidResourcesCode {
			t.Errorf("%s: create = %d %s, want 400 %s", name, code, body, invalidResourcesCode)
		}
	}
	if code, body := createQuotaApp(t, rt, cookie, "ok", proj, `,"resources":{"shm_size_bytes":1073741824}`); code != http.StatusCreated {
		t.Fatalf("shm within the cap = %d %s, want 201", code, body)
	}
	put := `{"name":"ok","image":"levelrail/web:1","port":3000,"resources":{"shm_size_bytes":1000000000000000}}`
	if code, body := quotaDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/ok", put); code != http.StatusBadRequest || !strings.Contains(body, invalidResourcesCode) {
		t.Fatalf("PUT huge shm = %d %s, want 400", code, body)
	}
	used, err := db.QuotaUsage(context.Background(), quota.ScopeProject, proj)
	if err != nil {
		t.Fatal(err)
	}
	if used.MemoryBytes < 0 || used.NanoCPUs < 0 {
		t.Fatalf("usage went negative: %+v", used)
	}
}
