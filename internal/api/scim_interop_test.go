package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestDropUnsupportedSCIMUserPatch(t *testing.T) {
	cases := []struct {
		name, in, want string
		dropped        int
	}{
		{"supported only", `{"Operations":[{"op":"replace","path":"displayName","value":"A"}]}`, "", 0},
		{"entra mail path", `{"Operations":[{"op":"Replace","path":"emails[type eq \"work\"].value","value":"a@b.c"},{"op":"Replace","path":"active","value":"True"}]}`, `[{"op":"Replace","path":"active","value":"True"}]`, 1},
		{"enterprise and title", `{"Operations":[{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":"x"},{"op":"Add","path":"title","value":"t"}]}`, `[]`, 2},
		{"pathless keys", `{"Operations":[{"op":"replace","value":{"title":"t","displayName":"D"}}]}`, `[{"op":"replace","value":{"displayName":"D"}}]`, 1},
		{"pathless all unsupported", `{"Operations":[{"op":"replace","value":{"title":"t"}}]}`, `[]`, 1},
		{"name subpath", `{"Operations":[{"op":"replace","path":"name.givenName","value":"G"}]}`, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, dropped := dropUnsupportedSCIMUserPatch([]byte(tc.in))
			if len(dropped) != tc.dropped {
				t.Fatalf("dropped %v, want %d", dropped, tc.dropped)
			}
			if tc.want == "" {
				if string(out) != tc.in {
					t.Fatalf("body changed: %s", out)
				}
				return
			}
			var res struct {
				Operations json.RawMessage `json:"Operations"`
			}
			if err := json.Unmarshal(out, &res); err != nil {
				t.Fatal(err)
			}
			var got, want any
			_ = json.Unmarshal(res.Operations, &got)
			_ = json.Unmarshal([]byte(tc.want), &want)
			g, _ := json.Marshal(got)
			w, _ := json.Marshal(want)
			if string(g) != string(w) {
				t.Fatalf("operations = %s, want %s", g, w)
			}
		})
	}
}

func TestSCIMPatchWithUnsupportedAttributesStillApplies(t *testing.T) {
	h := newSCIMHarness(t)
	id := h.createUser("ada@example.com")
	rec := h.patch("/Users/"+id, `[{"op":"Replace","path":"emails[type eq \"work\"].value","value":"x@example.com"},{"op":"Replace","path":"title","value":"CTO"},{"op":"Add","path":"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department","value":"Eng"},{"op":"Replace","path":"displayName","value":"Ada L"}]`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ada L") {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body)
	}
	rec = h.patch("/Users/"+id, `[{"op":"replace","value":{"title":"x","displayName":"Ada M"}}]`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Ada M") {
		t.Fatalf("pathless patch: %d %s", rec.Code, rec.Body)
	}
}

func TestSCIMDeactivateAnswersInactiveAndPatchReactivates(t *testing.T) {
	h := newSCIMHarness(t)
	ctx := context.Background()
	id := h.createUser("ada@example.com")
	user := h.platformUser(id)
	rec := h.patch("/Users/"+id, `[{"op":"replace","path":"active","value":false}]`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active":false`) {
		t.Fatalf("deactivate answer: %d %s", rec.Code, rec.Body)
	}
	if !h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("deactivation left the account enabled")
	}
	rec = h.patch("/Users/"+id, `[{"op":"Replace","path":"active","value":"True"}]`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"active":true`) {
		t.Fatalf("reactivate: %d %s", rec.Code, rec.Body)
	}
	if h.rt.accountDisabled(ctx, user.ID) {
		t.Fatal("reactivation left the account disabled")
	}
	if rec := h.do(http.MethodGet, "/Users/"+id, h.token, ""); rec.Code != http.StatusOK {
		t.Fatalf("reactivated user not visible: %d", rec.Code)
	}
}

func TestSCIMOktaFilteredMemberRemoval(t *testing.T) {
	h := newSCIMHarness(t)
	a, b := h.createUser("ma@example.com"), h.createUser("mb@example.com")
	g := h.createGroup("Eng", a, b)
	rec := h.patch("/Groups/"+g, `[{"op":"remove","path":"members[value eq \"`+a+`\"]"}]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("filtered removal: %d %s", rec.Code, rec.Body)
	}
	if got := h.groupMembers(g); len(got) != 1 || got[0] != b {
		t.Fatalf("members = %v, want only %s", got, b)
	}
}

func TestSCIMUnknownIDIsNotFoundAndCreateSetsLocation(t *testing.T) {
	h := newSCIMHarness(t)
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		if rec := h.do(method, "/Users/not-an-id", h.token, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("%s unknown id: %d %s", method, rec.Code, rec.Body)
		}
	}
	rec := h.do(http.MethodPost, "/Users", h.token, `{"schemas":["`+testSCIMUserSchema+`"],"userName":"grace@example.com","emails":[{"value":"grace@example.com","primary":true}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	id, err := scimResponseID(rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if loc := rec.Header().Get("Location"); !strings.HasSuffix(loc, scimBasePath+"/Users/"+id) {
		t.Fatalf("Location = %q", loc)
	}
}
