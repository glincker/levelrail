package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var liveEncCertRe = regexp.MustCompile(`(?s)use="encryption".*?<(?:ds:)?X509Certificate[^>]*>(.*?)</`)

func (k *liveKeycloak) enableEncryption(t *testing.T, spEntityID, spMetadata string) {
	t.Helper()
	m := liveEncCertRe.FindStringSubmatch(spMetadata)
	if m == nil {
		t.Fatalf("no encryption certificate in SP metadata: %.1800s", spMetadata)
	}
	_, lb, _ := k.admin(t, http.MethodGet, "/"+k.realm+"/clients?clientId="+url.QueryEscape(spEntityID), nil)
	var cs []map[string]any
	if err := json.Unmarshal(lb, &cs); err != nil || len(cs) != 1 {
		t.Fatalf("find client: %v %s", err, lb)
	}
	attrs, _ := cs[0]["attributes"].(map[string]any)
	attrs["saml.encrypt"] = "true"
	attrs["saml.encryption.certificate"] = strings.Join(strings.Fields(m[1]), "")
	cs[0]["attributes"] = attrs
	id, _ := cs[0]["id"].(string)
	if code, b, _ := k.admin(t, http.MethodPut, "/"+k.realm+"/clients/"+id, cs[0]); code != http.StatusNoContent {
		t.Fatalf("update client: %d %s", code, b)
	}
}

// The sign-in library looks for an assertion signature in the raw response, which
// an encrypted assertion hides, so encrypted assertions are a documented refusal.
func TestLiveSAMLEncryptedAssertionIsRefusedCleanly(t *testing.T) {
	skipUnlessLiveIDP(t)
	kc := newLiveKeycloak(t)
	h := newLiveHarness(t, kc, liveSignAssertion, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, samlPathPrefix+h.connID+"/metadata", nil)
	req.TLS = &tls.ConnectionState{}
	h.rt.Handler().ServeHTTP(rec, req)
	kc.enableEncryption(t, samlTestBase+samlPathPrefix+h.connID+"/metadata", rec.Body.String())
	flow, resp := h.liveSignIn(kc, "ada", "ada-pass")
	if raw, _ := base64.StdEncoding.DecodeString(resp); !bytes.Contains(raw, []byte("EncryptedAssertion")) {
		t.Fatal("keycloak did not encrypt the assertion")
	}
	wantSAMLError(t, h.post(flow, resp), samlErrInvalid)
	wantLiveReason(t, "assertion not signed")
	if _, err := h.db.GetUserByEmail(context.Background(), "ada@example.com"); err == nil {
		t.Fatal("a refused encrypted sign-in left an account behind")
	}
}
