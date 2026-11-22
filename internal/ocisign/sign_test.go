package ocisign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var authRe = regexp.MustCompile(`^Signature version="1",keyId="([^"]+)",algorithm="rsa-sha256",headers="([^"]+)",signature="([^"]+)"$`)

// newFake rejects any request whose signature does not verify with pub.
func newFake(t *testing.T, pub *rsa.PublicKey, keyID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := authRe.FindStringSubmatch(r.Header.Get("Authorization"))
		if m == nil || m[1] != keyID {
			http.Error(w, "bad auth", http.StatusUnauthorized)
			return
		}
		if m[2] != strings.Join(Headers(r.Method), " ") {
			http.Error(w, "bad header list", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if hasBody(r.Method) {
			sum := sha256.Sum256(body)
			if r.Header.Get("X-Content-Sha256") != base64.StdEncoding.EncodeToString(sum[:]) ||
				r.Header.Get("Content-Length") != itoa(len(body)) || r.Header.Get("Content-Type") == "" {
				http.Error(w, "bad body headers", http.StatusUnauthorized)
				return
			}
		}
		sig, err := base64.StdEncoding.DecodeString(m[3])
		if err != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		digest := sha256.Sum256([]byte(SigningString(r)))
		if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
			http.Error(w, "signature mismatch", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestSignerAgainstVerifyingServer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	id := KeyID("ocid1.tenancy.oc1..t", "ocid1.user.oc1..u", "aa:bb")
	srv := newFake(t, &key.PublicKey, id)
	defer srv.Close()
	s := Signer{KeyID: id, Key: key, Now: time.Now, AllowInsecure: true}

	cases := []struct {
		method string
		body   string
	}{
		{http.MethodGet, ""}, {http.MethodDelete, ""},
		{http.MethodPost, `{"a":1}`}, {http.MethodPut, `{"b":"x y"}`}, {http.MethodPost, ""},
	}
	for _, c := range cases {
		req, _ := http.NewRequest(c.method, srv.URL+"/20160918/instances?x=a%20b&y=1", nil)
		if err := s.Sign(req, []byte(c.body)); err != nil {
			t.Fatalf("%s sign: %v", c.method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s body=%q got %d", c.method, c.body, resp.StatusCode)
		}
	}

	// Unsigned, tampered body and wrong key must all be rejected.
	un, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	post, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", nil)
	_ = s.Sign(post, []byte(`{"a":1}`))
	post.Body = io.NopCloser(bytes.NewReader([]byte(`{"a":2}`)))
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	bad, _ := http.NewRequest(http.MethodPut, srv.URL+"/x", nil)
	_ = Signer{KeyID: id, Key: other, AllowInsecure: true}.Sign(bad, []byte("{}"))
	for name, r := range map[string]*http.Request{"unsigned": un, "tampered": post, "wrongkey": bad} {
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s accepted with %d", name, resp.StatusCode)
		}
	}
}

func TestSignerRefusesPlainHTTPAndBodyOnGet(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	req, _ := http.NewRequest(http.MethodGet, "http://iaas.example/x", nil)
	if err := (Signer{KeyID: "k", Key: key}).Sign(req, nil); err == nil {
		t.Fatal("plain http was signed")
	}
	req, _ = http.NewRequest(http.MethodGet, "https://iaas.example/x", nil)
	if err := (Signer{KeyID: "k", Key: key}).Sign(req, []byte("x")); err == nil {
		t.Fatal("GET with body was signed")
	}
}

func TestSessionKeyIDSignsAndVerifies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	id := SessionKeyID("hdr.payload.sig")
	if id != "ST$hdr.payload.sig" {
		t.Fatalf("keyId = %q", id)
	}
	srv := newFake(t, &key.PublicKey, id)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/20160918/instances", nil)
	if err := (Signer{KeyID: id, Key: key, AllowInsecure: true}).Sign(req, nil); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestSignRefusesUnsafeKeyID(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	for _, id := range []string{`ST$a"b`, "ST$a,b", "ST$a b", "ST$a\nb"} {
		req := httptest.NewRequest(http.MethodGet, "https://example.test/x", nil)
		if err := (Signer{KeyID: id, Key: key}).Sign(req, nil); err == nil {
			t.Fatalf("key id %q was accepted", id)
		}
	}
}
