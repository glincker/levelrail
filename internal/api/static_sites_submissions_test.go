package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestStaticSiteSubmissionsListAndDelete(t *testing.T) {
	rt, _, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"contact"}`)
	rec := publicPost(rt, f.EndpointPath, "application/json", `{"name":"Ada","message":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("submit = %d: %s", rec.Code, rec.Body)
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/web/submissions", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	var got siteSubmissionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Submissions) != 1 || got.Submissions[0].Form != "contact" || got.Submissions[0].Fields["name"] != "Ada" {
		t.Fatalf("submissions = %+v", got.Submissions)
	}
	id := got.Submissions[0].ID
	path := "/api/v1/static-sites/web/submissions/contact/" + strings.TrimSpace(jsonInt(id))
	if rec = serve(rt, authedRequest(t, cookie, http.MethodDelete, path, "")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d: %s", rec.Code, rec.Body)
	}
	if rec = serve(rt, authedRequest(t, cookie, http.MethodDelete, path, "")); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func jsonInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
