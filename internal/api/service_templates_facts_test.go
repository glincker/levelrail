package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func listTemplates(t *testing.T, query string) (int, []serviceTemplateListItem) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/service-templates"+query, ""))
	var got []serviceTemplateListItem
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if strings.Contains(rec.Body.String(), "hosted_use") {
			t.Error("response exposes hosted_use")
		}
	}
	return rec.Code, got
}

func TestServiceTemplatesFacts_Filters(t *testing.T) {
	_, all := listTemplates(t, "")
	if len(all) != len(availableTemplates()) {
		t.Fatalf("unfiltered list hid templates: %d of %d", len(all), len(availableTemplates()))
	}
	withTrust := 0
	for _, it := range all {
		if it.Trust != nil && it.Listing != nil {
			withTrust++
		}
	}
	if withTrust != len(all) {
		t.Errorf("%d of %d items carry trust and listing", withTrust, len(all))
	}

	_, permissive := listTemplates(t, "?license_class=permissive")
	if len(permissive) == 0 || len(permissive) >= len(all) {
		t.Fatalf("permissive = %d of %d, want a strict subset", len(permissive), len(all))
	}
	for _, it := range permissive {
		if it.License == nil || it.License.Class != "permissive" {
			t.Errorf("%s leaked through license_class filter: %+v", it.ID, it.License)
		}
	}

	_, verified := listTemplates(t, "?verified=true")
	for _, it := range verified {
		if it.Trust.Level == "listed" {
			t.Errorf("%s is only listed but passed verified=true", it.ID)
		}
	}

	_, arm := listTemplates(t, "?arch=arm64")
	for _, it := range arm {
		if s := it.Arch["arm64"]; s != "tested" && s != "published" {
			t.Errorf("%s arm64 = %q passed arch=arm64", it.ID, s)
		}
	}

	_, active := listTemplates(t, "?include_deprecated=false")
	for _, it := range active {
		if it.Status != "" && it.Status != "active" {
			t.Errorf("%s (%s) passed include_deprecated=false", it.ID, it.Status)
		}
	}

	for _, bad := range []string{"?license_class=weird", "?arch=riscv", "?verified=maybe"} {
		if code, _ := listTemplates(t, bad); code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", bad, code)
		}
	}
}
