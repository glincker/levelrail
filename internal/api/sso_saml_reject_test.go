package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSAMLRejectsWrongAudienceExpiredAndUnsigned(t *testing.T) {
	h := newSAMLHarness(t, nil)

	id, flow := h.start()
	tg := h.target(id)
	a := h.idp.Assertion(tg, "nid-aud", map[string][]string{"email": {"aud@example.com"}}, time.Now())
	a.Conditions.AudienceRestrictions[0].Audience.Value = "https://other-sp.example.test/metadata"
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, a, h.idp.DefaultOptions(tg))), samlErrInvalid)

	id, flow = h.start()
	tg = h.target(id)
	a = h.idp.Assertion(tg, "nid-aud", map[string][]string{"email": {"aud@example.com"}}, time.Now())
	a.Conditions.AudienceRestrictions = nil
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, a, h.idp.DefaultOptions(tg))), samlErrInvalid)

	id, flow = h.start()
	tg = h.target(id)
	old := h.idp.Assertion(tg, "nid-old", map[string][]string{"email": {"old@example.com"}}, time.Now().Add(-time.Hour))
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, old, h.idp.DefaultOptions(tg))), samlErrInvalid)

	// Response signed, assertion not: the strict gate passes it and the library refuses it.
	id, flow = h.start()
	tg = h.target(id)
	o := h.idp.DefaultOptions(tg)
	o.SignAssert = false
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, h.idp.Assertion(tg, "nid-u", map[string][]string{"email": {"u@example.com"}}, time.Now()), o)), samlErrInvalid)

	id, flow = h.start()
	tg = h.target(id)
	o = h.idp.DefaultOptions(tg)
	o.SignAssert, o.SignResponse = false, false
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, h.idp.Assertion(tg, "nid-u", map[string][]string{"email": {"u@example.com"}}, time.Now()), o)), samlErrInvalid)

	forger := newTestIDP(t, samlTestIDP)
	id, flow = h.start()
	tg = h.target(id)
	forged := forger.Respond(t, forger.Assertion(tg, "nid-x", map[string][]string{"email": {"x@example.com"}}, time.Now()), forger.DefaultOptions(tg))
	wantSAMLError(t, h.post(flow, forged), samlErrInvalid)

	if _, err := h.db.GetUserByEmail(context.Background(), "u@example.com"); err == nil {
		t.Fatal("a refused sign-in left a platform user behind")
	}
}

func TestSAMLRefusesReplayAndIdPInitiated(t *testing.T) {
	h := newSAMLHarness(t, nil)
	id, flow := h.start()
	resp := h.respond(id, "nid-1", "one@example.com")
	wantSAMLSession(t, h, h.post(flow, resp))
	wantSAMLError(t, h.post(flow, resp), samlErrInvalidState)

	// The captured response under a fresh flow answers a different request.
	_, fresh := h.start()
	wantSAMLError(t, h.post(fresh, resp), samlErrInvalid)

	// IdP-initiated: no request at all, with and without a flow cookie.
	unsolicited := h.respond("", "nid-1", "one@example.com")
	wantSAMLError(t, h.post(nil, unsolicited), samlErrInvalidState)
	_, flow = h.start()
	wantSAMLError(t, h.post(flow, unsolicited), samlErrInvalid)

	// A bearer confirmation for another request is refused even when the response wrapper matches.
	id, flow = h.start()
	tg := h.target(id)
	a := h.idp.Assertion(h.target("id-not-this-one"), "nid-1", map[string][]string{"email": {"one@example.com"}}, time.Now())
	wantSAMLError(t, h.post(flow, h.idp.Respond(t, a, h.idp.DefaultOptions(tg))), samlErrInvalid)
}

func TestSAMLPolicyRefusals(t *testing.T) {
	h := newSAMLHarness(t, map[string]any{"jit_provisioning": false, "allowed_domains": []string{"example.com"}})
	id, flow := h.start()
	wantSAMLError(t, h.post(flow, h.respond(id, "nid-new", "new@example.com")), samlErrNotProvisioned)
	id, flow = h.start()
	wantSAMLError(t, h.post(flow, h.respond(id, "nid-evil", "evil@other.test")), samlErrDomain)

	plain := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/login", nil)
	plainRec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(plainRec, plain)
	wantSAMLError(t, plainRec, samlErrUnavailable)
}

// The library's own SAML routes accept IdP-initiated responses; the platform must never expose them.
func TestLibrarySAMLRoutesAreNotReachable(t *testing.T) {
	h := newSAMLHarness(t, nil)
	lib := h.libID()
	prefix := h.rt.authEngine.Prefix()
	for _, p := range []string{"/saml/" + lib + "/acs", "/saml/" + lib + "/login", "/saml/" + lib + "/metadata", "/orgs"} {
		for _, m := range []string{http.MethodGet, http.MethodPost} {
			rec := httptest.NewRecorder()
			h.rt.Handler().ServeHTTP(rec, httptest.NewRequest(m, prefix+p, nil))
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", m, prefix+p, rec.Code)
			}
		}
	}
}
