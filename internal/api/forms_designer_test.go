package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestFormDesignerFlow(t *testing.T) {
	rt, _, cookie := formsTestRouter(t)
	f := createTestForm(t, rt, cookie, `{"name":"contact"}`)
	if f.EndpointSource != endpointSourceLocal {
		t.Fatalf("endpoint source = %q, want local with no public URL", f.EndpointSource)
	}

	schema := `{"fields":[{"name":"email","type":"email","required":true},{"name":"note","type":"text","max_length":5}],"success_message":"Got it","allowed_origins":["https://site.test"]}`
	rec := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/forms/contact/fields", schema))
	if rec.Code != http.StatusOK {
		t.Fatalf("set fields = %d: %s", rec.Code, rec.Body)
	}
	bad := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/forms/contact/fields", `{"fields":[{"name":"a","type":"nope"}]}`))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad schema = %d", bad.Code)
	}

	form := "application/x-www-form-urlencoded"
	if rec := publicPost(rt, f.EndpointPath, form, "email=nope", "Origin", "https://site.test"); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d: %s", rec.Code, rec.Body)
	}
	if rec := publicPost(rt, f.EndpointPath, form, "email=a%40b.co", "Origin", "https://evil.test"); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong origin = %d", rec.Code)
	}
	ok := publicPost(rt, f.EndpointPath, form, "email=a%40b.co&junk=1", "Origin", "https://site.test", "Accept", "application/json")
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), "Got it") || ok.Header().Get("Access-Control-Allow-Origin") != "https://site.test" {
		t.Fatalf("ok = %d %s cors=%q", ok.Code, ok.Body, ok.Header().Get("Access-Control-Allow-Origin"))
	}

	test := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/forms/contact/test", ``))
	if test.Code != http.StatusOK {
		t.Fatalf("test = %d: %s", test.Code, test.Body)
	}
	list := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/forms/contact/submissions", ""))
	var page formSubmissionsResponse
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil || len(page.Submissions) != 2 {
		t.Fatalf("submissions = %s", list.Body)
	}
	if !page.Submissions[0].Test || page.Submissions[1].Test {
		t.Fatalf("test flags wrong: %+v", page.Submissions)
	}
	if _, junk := page.Submissions[1].Fields["junk"]; junk {
		t.Fatal("undeclared field was stored")
	}
}
