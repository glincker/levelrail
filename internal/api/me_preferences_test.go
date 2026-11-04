package api

import (
	"net/http"
	"strings"
	"testing"
)

const mePrefPath = "/api/v1/me/preferences/appearance"

func TestMePreferencesAppearance_RoundTripOwnUserAndReset(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	other := accountOtherUser(t, h)

	rec := h.send(http.MethodGet, mePrefPath, "", "", own)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"value":null}` {
		t.Fatalf("unset get = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.send(http.MethodPut, mePrefPath+"?user_id=user_other", `{"theme":"dark","density":2,"x":{"on":true},"user_id":"user_other"}`, "", own)
	if rec.Code != http.StatusOK {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.send(http.MethodGet, mePrefPath, "", "", own)
	if !strings.Contains(rec.Body.String(), `"theme":"dark"`) {
		t.Fatalf("own get = %s", rec.Body.String())
	}
	rec = h.send(http.MethodGet, mePrefPath, "", "", other)
	if strings.TrimSpace(rec.Body.String()) != `{"value":null}` {
		t.Fatalf("other user sees %s", rec.Body.String())
	}
	if rec = h.send(http.MethodDelete, mePrefPath, "", "", own); rec.Code != http.StatusOK {
		t.Fatalf("delete = %d", rec.Code)
	}
	rec = h.send(http.MethodGet, mePrefPath, "", "", own)
	if strings.TrimSpace(rec.Body.String()) != `{"value":null}` {
		t.Fatalf("after reset = %s", rec.Body.String())
	}
	if rec = h.send(http.MethodGet, mePrefPath, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", rec.Code)
	}
}

func TestMePreferencesAppearance_RejectsBadBodies(t *testing.T) {
	h := newCodeHarness(t, false)
	own := h.userSession()
	cases := map[string]struct {
		body string
		want int
	}{
		"array":      {`[1]`, http.StatusBadRequest},
		"null leaf":  {`{"a":null}`, http.StatusBadRequest},
		"array leaf": {`{"a":[1]}`, http.StatusBadRequest},
		"too deep":   {`{"a":{"b":{"c":{"d":{"e":1}}}}}`, http.StatusBadRequest},
		"four ok":    {`{"a":{"b":{"c":{"d":1}}}}`, http.StatusOK},
		"trailing":   {`{"a":1} {"b":2}`, http.StatusBadRequest},
		"oversize":   {`{"a":"` + strings.Repeat("x", 9000) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for name, c := range cases {
		if rec := h.send(http.MethodPut, mePrefPath, c.body, "", own); rec.Code != c.want {
			t.Errorf("%s = %d %s, want %d", name, rec.Code, rec.Body.String(), c.want)
		}
	}
}

func TestMePreferencesAppearance_TokenWithoutUserIsRefused(t *testing.T) {
	h := newCodeHarness(t, false)
	plain := seedOwnedToken(t, h, "tok_pref_nouser", "", []string{AbilityRead, AbilityWrite})
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		rec := serve(h.rt, bearerRequest(m, mePrefPath, `{"a":1}`, plain))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s ownerless token = %d %s", m, rec.Code, rec.Body.String())
		}
	}
}
