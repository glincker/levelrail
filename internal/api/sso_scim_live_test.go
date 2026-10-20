package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testSCIMGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"

type scimList struct {
	TotalResults int `json:"totalResults"`
	StartIndex   int `json:"startIndex"`
	ItemsPerPage int `json:"itemsPerPage"`
	Resources    []struct {
		ID          string `json:"id"`
		UserName    string `json:"userName"`
		DisplayName string `json:"displayName"`
		Members     []struct {
			Value string `json:"value"`
		} `json:"members"`
	} `json:"Resources"`
}

func (h *scimHarness) list(path string) (int, scimList) {
	h.t.Helper()
	rec := h.do(http.MethodGet, path, h.token, "")
	var l scimList
	_ = json.Unmarshal(rec.Body.Bytes(), &l)
	return rec.Code, l
}

func (h *scimHarness) scimType(rec *httptest.ResponseRecorder) string {
	var e struct {
		ScimType string `json:"scimType"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	return e.ScimType
}

func (h *scimHarness) createGroup(name string, members ...string) string {
	h.t.Helper()
	var ms []string
	for _, m := range members {
		ms = append(ms, `{"value":"`+m+`"}`)
	}
	rec := h.do(http.MethodPost, "/Groups", h.token, `{"schemas":["`+testSCIMGroupSchema+`"],"displayName":"`+name+`","members":[`+strings.Join(ms, ",")+`]}`)
	if rec.Code != http.StatusCreated {
		h.t.Fatalf("create group: %d %s", rec.Code, rec.Body)
	}
	id, err := scimResponseID(rec.Body.Bytes())
	if err != nil {
		h.t.Fatal(err)
	}
	return id
}

func (h *scimHarness) groupMembers(id string) []string {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/Groups/"+id, h.token, "")
	var g struct {
		Members []struct {
			Value string `json:"value"`
		} `json:"members"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &g) != nil {
		h.t.Fatalf("get group: %d %s", rec.Code, rec.Body)
	}
	var out []string
	for _, m := range g.Members {
		out = append(out, m.Value)
	}
	return out
}

func TestLiveSCIMConformance(t *testing.T) {
	skipUnlessLiveIDP(t)
	h := newSCIMHarness(t)
	ctx := context.Background()

	t.Run("authentication", func(t *testing.T) {
		for name, tok := range map[string]string{"none": "", "forged": "forged-token-value"} {
			if rec := h.do(http.MethodGet, "/Users", tok, ""); rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s token: %d", name, rec.Code)
			}
		}
		tok, id := h.mint("short-lived")
		if rec := h.do(http.MethodGet, "/Users", tok, ""); rec.Code != http.StatusOK {
			t.Fatalf("fresh token: %d", rec.Code)
		}
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, authedRequest(t, h.admin, http.MethodDelete, "/api/v1/sso/scim/tokens/"+id, ""))
		if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
		}
		if rec := h.do(http.MethodGet, "/Users", tok, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked token still works: %d", rec.Code)
		}
	})

	t.Run("discovery documents", func(t *testing.T) {
		rec := h.do(http.MethodGet, "/ServiceProviderConfig", h.token, "")
		var spc struct {
			Patch  struct{ Supported bool } `json:"patch"`
			Bulk   struct{ Supported bool } `json:"bulk"`
			Filter struct {
				Supported  bool `json:"supported"`
				MaxResults int  `json:"maxResults"`
			} `json:"filter"`
			Etag struct{ Supported bool } `json:"etag"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &spc); err != nil || !spc.Patch.Supported || spc.Bulk.Supported || !spc.Filter.Supported || spc.Etag.Supported {
			t.Fatalf("service provider config: %v %s", err, rec.Body)
		}
		for _, p := range []string{"/ResourceTypes", "/ResourceTypes/User", "/Schemas", "/Schemas/" + url.PathEscape(testSCIMUserSchema)} {
			if rec := h.do(http.MethodGet, p, h.token, ""); rec.Code != http.StatusOK {
				t.Fatalf("GET %s: %d %s", p, rec.Code, rec.Body)
			}
		}
		if rec := h.do(http.MethodPost, "/Bulk", h.token, `{}`); rec.Code != http.StatusNotFound {
			t.Fatalf("bulk: %d", rec.Code)
		}
	})

	t.Run("user create, read, filter and page", func(t *testing.T) {
		ids := make([]string, 0, 5)
		for i := 0; i < 5; i++ {
			ids = append(ids, h.createUser(fmt.Sprintf("page%d@example.com", i)))
		}
		rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"PAGE0@example.com","emails":[{"value":"PAGE0@example.com"}]}`)
		if rec.Code != http.StatusConflict || h.scimType(rec) != "uniqueness" {
			t.Fatalf("duplicate: %d %s", rec.Code, rec.Body)
		}
		if rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"]}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("no userName: %d", rec.Code)
		}
		if rec := h.do(http.MethodPost, "/Users", h.token, `{`); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad json: %d", rec.Code)
		}
		if rec := h.do(http.MethodGet, "/Users/"+ids[0], h.token, ""); rec.Code != http.StatusOK || rec.Header().Get("ETag") != "" {
			t.Fatalf("get: %d etag=%q", rec.Code, rec.Header().Get("ETag"))
		}
		if rec := h.do(http.MethodGet, "/Users/01M5BFJ67T249AH5W3V9TE8C6J", h.token, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown id: %d", rec.Code)
		}
		code, l := h.list("/Users?startIndex=2&count=2")
		if code != http.StatusOK || l.TotalResults < 5 || l.StartIndex != 2 || l.ItemsPerPage != 2 || len(l.Resources) != 2 {
			t.Fatalf("page: %d %+v", code, l)
		}
		for _, q := range []string{`userName eq "page3@example.com"`, `userName eq "PAGE3@EXAMPLE.COM"`, `externalId eq "ext-page3@example.com"`, `emails.value eq "page3@example.com"`} {
			code, l := h.list("/Users?filter=" + url.QueryEscape(q))
			if code != http.StatusOK || l.TotalResults != 1 || l.Resources[0].ID != ids[3] {
				t.Fatalf("filter %q: %d %+v", q, code, l)
			}
		}
		code, l = h.list("/Users?filter=" + url.QueryEscape(`userName eq "nobody@example.com"`))
		if code != http.StatusOK || l.TotalResults != 0 {
			t.Fatalf("empty filter: %d %+v", code, l)
		}
		for _, q := range []string{`userName co "page"`, `userName eq "a" and active eq true`, `((`} {
			rec := h.do(http.MethodGet, "/Users?filter="+url.QueryEscape(q), h.token, "")
			if rec.Code != http.StatusBadRequest || h.scimType(rec) != "invalidFilter" {
				t.Fatalf("unsupported filter %q: %d %s", q, rec.Code, rec.Body)
			}
		}
		if rec := h.do(http.MethodPut, "/Users/"+ids[0], h.token, `{}`); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("put: %d", rec.Code)
		}
	})

	t.Run("okta and entra shaped deprovisioning", func(t *testing.T) {
		for name, ops := range map[string]string{
			"okta bool":  `[{"op":"replace","path":"active","value":false}]`,
			"entra text": `[{"op":"Replace","path":"active","value":"False"}]`,
			"pathless":   `[{"op":"replace","value":{"active":false}}]`,
		} {
			id := h.createUser(strings.ReplaceAll(name, " ", "-") + "@example.com")
			user := h.platformUser(id)
			rec := h.patch("/Users/"+id, ops)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active":false`) {
				t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
			}
			if !h.rt.accountDisabled(ctx, user.ID) {
				t.Fatalf("%s: account still enabled", name)
			}
			if rec := h.patch("/Users/"+id, `[{"op":"Replace","path":"active","value":"True"}]`); rec.Code != http.StatusOK || h.rt.accountDisabled(ctx, user.ID) {
				t.Fatalf("%s reactivate: %d", name, rec.Code)
			}
			if rec := h.do(http.MethodDelete, "/Users/"+id, h.token, ""); rec.Code != http.StatusNoContent || !h.rt.accountDisabled(ctx, user.ID) {
				t.Fatalf("%s delete: %d", name, rec.Code)
			}
			if rec := h.do(http.MethodGet, "/Users/"+id, h.token, ""); rec.Code != http.StatusNotFound {
				t.Fatalf("%s: deleted user visible: %d", name, rec.Code)
			}
		}
	})

	t.Run("groups drive roles", func(t *testing.T) {
		mp := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(mp, authedRequest(t, h.admin, http.MethodPut, "/api/v1/sso/group-mappings", `{"mappings":[{"group":"Eng","role_id":"role_operator","priority":5}]}`))
		if mp.Code != http.StatusOK {
			t.Fatalf("mappings: %d %s", mp.Code, mp.Body)
		}
		a, b := h.createUser("ga@example.com"), h.createUser("gb@example.com")
		g := h.createGroup("Eng", a)
		role := func(engineID string) string {
			r, _ := h.db.UserRoleID(ctx, h.platformUser(engineID).ID)
			return r
		}
		if got := h.groupMembers(g); len(got) != 1 || got[0] != a || role(a) != "role_operator" || role(b) != "role_viewer" {
			t.Fatalf("after create: members=%v roles=%s,%s", got, role(a), role(b))
		}
		if rec := h.patch("/Groups/"+g, `[{"op":"add","path":"members","value":[{"value":"`+b+`"}]}]`); rec.Code != http.StatusOK || role(b) != "role_operator" {
			t.Fatalf("add member: %d %s role=%s", rec.Code, rec.Body, role(b))
		}
		if rec := h.patch("/Groups/"+g, `[{"op":"remove","path":"members","value":[{"value":"`+a+`"}]}]`); rec.Code != http.StatusOK || role(a) != "role_viewer" {
			t.Fatalf("remove member: %d %s role=%s", rec.Code, rec.Body, role(a))
		}
		if rec := h.patch("/Groups/"+g, `[{"op":"replace","path":"members","value":[{"value":"`+a+`"}]}]`); rec.Code != http.StatusOK || role(a) != "role_operator" || role(b) != "role_viewer" {
			t.Fatalf("replace members: %d %s", rec.Code, rec.Body)
		}
		if rec := h.patch("/Groups/"+g, `[{"op":"replace","path":"displayName","value":"Engineering"}]`); rec.Code != http.StatusOK || role(a) != "role_viewer" {
			t.Fatalf("rename group: %d %s role(a)=%s", rec.Code, rec.Body, role(a))
		}
		code, l := h.list("/Groups?filter=" + url.QueryEscape(`displayName eq "Engineering"`))
		if code != http.StatusOK || l.TotalResults != 1 {
			t.Fatalf("group filter: %d %+v", code, l)
		}
		if rec := h.patch("/Groups/"+g, `[{"op":"remove","path":"members[value eq \"`+a+`\"]"}]`); rec.Code != http.StatusOK || len(h.groupMembers(g)) != 0 {
			t.Fatalf("okta filtered member removal: %d %s members=%v", rec.Code, rec.Body, h.groupMembers(g))
		}
		if rec := h.do(http.MethodDelete, "/Groups/"+g, h.token, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("delete group: %d %s", rec.Code, rec.Body)
		}
		if rec := h.do(http.MethodGet, "/Groups/"+g, h.token, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("deleted group visible: %d", rec.Code)
		}
		if rec := h.do(http.MethodPut, "/Groups/"+g, h.token, `{}`); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("put group: %d", rec.Code)
		}
	})
}
