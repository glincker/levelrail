package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDiffEnvKeys(t *testing.T) {
	got := diffEnvKeys(
		map[string]string{"KEEP": "1", "EDIT": "a", "GONE": "x"},
		map[string]string{"KEEP": "1", "EDIT": "b", "NEW": "n"},
	)
	want := []store.EnvChange{
		{Key: "EDIT", Action: store.EnvChangeChanged},
		{Key: "GONE", Action: store.EnvChangeRemoved},
		{Key: "NEW", Action: store.EnvChangeAdded},
	}
	if len(got) != len(want) {
		t.Fatalf("diffEnvKeys() = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("diffEnvKeys()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCheckAppEnv(t *testing.T) {
	svc := &store.DesiredService{
		Name: "web",
		Env: map[string]string{
			"EMPTY": " ", "Api_Key": "1", "API_KEY": "2", "DUP": "x", "FINE": "ok",
		},
		SecretEnv:   []store.SecretEnvRef{{Name: "DUP"}, {Name: "NEEDED", Required: true}, {Name: "SET_OK", Required: true}},
		DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "gone", Field: "url"}},
	}
	got := checkAppEnv(svc, map[string]bool{"SET_OK": true}, true, func(string) bool { return false })
	kinds := map[string]string{}
	for _, f := range got {
		kinds[f.Kind+":"+f.Key] = f.Fix.Type
	}
	for key, fix := range map[string]string{
		envFindingEmptyValue + ":EMPTY":              envFixAuto,
		envFindingShadowed + ":DUP":                  envFixAuto,
		envFindingCaseDuplicate + ":Api_Key":         envFixManual,
		envFindingCaseDuplicate + ":API_KEY":         envFixManual,
		envFindingMissingSecret + ":NEEDED":          envFixManual,
		envFindingDanglingDatabase + ":DATABASE_URL": envFixAuto,
	} {
		if kinds[key] != fix {
			t.Errorf("finding %s fix = %q, want %q (all: %v)", key, kinds[key], fix, kinds)
		}
	}
	if _, bad := kinds[envFindingMissingSecret+":SET_OK"]; bad {
		t.Error("a required secret with a stored value must not be reported")
	}
	if len(got) != 6 {
		t.Errorf("findings = %d, want 6: %v", len(got), kinds)
	}
}

func TestEnvHistoryAndFix(t *testing.T) {
	f := newEnvFixture(t)
	body := `{"name":"web","image":"img:v1","port":3000,"env":{"PORT":"3000","EMPTY":"","NEW":"1"}}`
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web", body, http.StatusOK)

	rec := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/history", "", http.StatusOK)
	var hist envHistoryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &hist); err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, c := range hist.Changes {
		actions[c.Key] = c.Action
	}
	if actions["NEW"] != store.EnvChangeAdded || actions["API_SECRET"] != store.EnvChangeRemoved || actions["EMPTY"] != store.EnvChangeAdded {
		t.Fatalf("history = %v", actions)
	}
	if strings.Contains(rec.Body.String(), plainSecretValue) {
		t.Fatal("history must never carry a value")
	}

	rec = f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web/env/check", "", http.StatusOK)
	if !strings.Contains(rec.Body.String(), envFindingEmptyValue) {
		t.Fatalf("check = %s, want an empty_value finding", rec.Body.String())
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/check/fix", `{"kind":"empty_value","key":"EMPTY"}`, http.StatusOK)
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Env["EMPTY"]; ok {
		t.Fatal("fix did not remove EMPTY")
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/check/fix", `{"kind":"empty_value","key":"EMPTY"}`, http.StatusConflict)
}
