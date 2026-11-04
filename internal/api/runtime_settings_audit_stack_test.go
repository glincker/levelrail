package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func settingAuditDetails(t *testing.T, db *store.DB) []map[string]any {
	t.Helper()
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, e := range entries {
		if e.Action != AuditActionSettingsChange {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(e.Detail), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestSettingChangeAuditStack(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	key := runtimesettings.KeyBindBlockCredentialDirs
	before := runtimesettings.Setting{Key: key, Value: json.RawMessage("true")}
	after := runtimesettings.Setting{Key: key, Value: json.RawMessage("false")}

	plain := authedRequest(t, cookie, http.MethodPut, rsBase+"/"+key, "")
	rt.recordSettingChange(plain, before, after)
	scoped := authedRequest(t, cookie, http.MethodPut, rsBase+"/"+key, "")
	scoped = scoped.WithContext(iac.WithStackScope(scoped.Context(), iac.StackScope{StackID: "stk_1", StackName: "edge", Revision: 7}))
	rt.recordSettingChange(scoped, before, after)
	secret := runtimesettings.Setting{Key: "x.secret", Sensitive: true, Value: json.RawMessage(`"hunter2"`)}
	rt.recordSettingChange(plain, secret, secret)

	rows := settingAuditDetails(t, db)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	var gotPlain, gotStack, gotSecret map[string]any
	for _, m := range rows {
		switch {
		case m["key"] == "x.secret":
			gotSecret = m
		case m["stack"] != nil:
			gotStack = m
		default:
			gotPlain = m
		}
	}
	if gotPlain == nil || gotPlain["revision"] != nil || gotPlain["token_id"] != nil {
		t.Fatalf("unscoped row changed: %v", gotPlain)
	}
	if gotStack == nil || gotStack["stack"] != "edge" || gotStack["revision"] != float64(7) {
		t.Fatalf("stack row = %v", gotStack)
	}
	if gotSecret == nil || gotSecret["old"] != "***" || gotSecret["new"] != "***" {
		t.Fatalf("sensitive value logged: %v", gotSecret)
	}
}
