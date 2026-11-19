package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
)

// fakeDistRegistry implements the distribution token challenge: every /v2 call without a
// valid bearer for the repository answers 401 with a Www-Authenticate pointing at the realm.
type fakeDistRegistry struct {
	t       *testing.T
	pub     *ecdsa.PublicKey
	service string
	realm   string
	mu      sync.Mutex
	blobs   map[string][]byte
	tags    map[string]string
}

type distAccess struct {
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

func (f *fakeDistRegistry) grants(tok, name, action string) bool {
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil || len(sig) != 64 {
		return false
	}
	sum := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if !ecdsa.Verify(f.pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return false
	}
	raw, _ := base64.RawURLEncoding.DecodeString(p[1])
	var c struct {
		Exp    int64        `json:"exp"`
		Aud    string       `json:"aud"`
		Access []distAccess `json:"access"`
	}
	if json.Unmarshal(raw, &c) != nil || c.Aud != f.service || time.Now().Unix() > c.Exp {
		return false
	}
	for _, a := range c.Access {
		for _, act := range a.Actions {
			if a.Name == name && act == action {
				return true
			}
		}
	}
	return false
}

func (f *fakeDistRegistry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v2/")
	i := strings.Index(rest, "/manifests/")
	if i < 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	name, ref := rest[:i], rest[i+len("/manifests/"):]
	action := "pull"
	if r.Method == http.MethodPut {
		action = "push"
	}
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !f.grants(tok, name, action) {
		scope := "repository:" + name + ":" + action
		if action == "push" {
			scope = "repository:" + name + ":push,pull"
		}
		w.Header().Set("Www-Authenticate", `Bearer realm="`+f.realm+`",service="`+f.service+`",scope="`+scope+`"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == http.MethodPut {
		body, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(body)
		d := "sha256:" + hex.EncodeToString(sum[:])
		f.blobs[name+"@"+d] = body
		f.tags[name+":"+ref] = d
		w.Header().Set("Docker-Content-Digest", d)
		w.WriteHeader(http.StatusCreated)
		return
	}
	d := ref
	if !strings.HasPrefix(ref, "sha256:") {
		d = f.tags[name+":"+ref]
	}
	body, ok := f.blobs[name+"@"+d]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Docker-Content-Digest", d)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

// pushThroughChallenge plays the builder: try, follow the 401 challenge to the realm with the job token, retry.
func pushThroughChallenge(t *testing.T, regURL, name, tag, jobToken string, body []byte) (status int, digest string) {
	t.Helper()
	target := regURL + "/v2/" + name + "/manifests/" + tag
	do := func(bearer string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut, target, strings.NewReader(string(body)))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	first := do("")
	if first.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated push must get a challenge, got %d", first.StatusCode)
	}
	ch := first.Header.Get("Www-Authenticate")
	attr := func(k string) string {
		_, after, _ := strings.Cut(ch, k+`="`)
		v, _, _ := strings.Cut(after, `"`)
		return v
	}
	q := url.Values{"service": {attr("service")}, "scope": {attr("scope")}}
	req, _ := http.NewRequest(http.MethodGet, attr("realm")+"?"+q.Encode(), nil)
	req.SetBasicAuth("levelrail", jobToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, ""
	}
	var tr struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		t.Fatal(err)
	}
	second := do(tr.Token)
	return second.StatusCode, second.Header.Get("Docker-Content-Digest")
}

func TestRegistryTokenModeEndToEnd(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := build.NewPushTokenIssuer([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	reg := &fakeDistRegistry{t: t, pub: &key.PublicKey, service: "levelrail-registry", blobs: map[string][]byte{}, tags: map[string]string{}}
	regSrv := httptest.NewServer(reg)
	defer regSrv.Close()
	host := strings.TrimPrefix(regSrv.URL, "http://")
	svc := &build.RegistryTokenService{Issuer: issuer, Key: key, Service: reg.service, TokenIssuer: "lr",
		BearerTTL: func() time.Duration { return time.Minute }, Host: func() string { return host }}
	rt := &Router{registryToken: svc, registryTokenActive: func() bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/registry/token", rt.handleRegistryToken)
	realmSrv := httptest.NewServer(mux)
	defer realmSrv.Close()
	reg.realm = realmSrv.URL + "/api/v1/registry/token"

	const name = "levelrail/app"
	job, err := issuer.Issue(host+"/"+name, "b1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schemaVersion":2,"layers":[]}`)

	// Push with the job token: challenge, realm exchange, push scope on its own repository only.
	status, digest := pushThroughChallenge(t, regSrv.URL, name, "b1", job, manifest)
	sum := sha256.Sum256(manifest)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if status != http.StatusCreated || digest != want {
		t.Fatalf("push: status %d digest %q want %q", status, digest, want)
	}
	if st, _ := pushThroughChallenge(t, regSrv.URL, "levelrail/other", "b1", job, manifest); st != http.StatusUnauthorized {
		t.Fatalf("a job token must not push to another repository, got %d", st)
	}

	// The control plane verifies the digest with a pull-only bearer, then pulls by digest.
	client := &build.RegistryClient{Insecure: true, Bearer: svc.MintPull}
	if err := build.VerifyPushedDigest(t.Context(), client, host+"/"+name+":b1", want); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := build.VerifyPushedDigest(t.Context(), client, host+"/"+name+":b1", "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatal("a digest the registry does not hold must fail verification")
	}
	tok, ok, err := svc.MintPullForImage(host + "/" + name + "@" + want)
	if err != nil || !ok {
		t.Fatalf("mint pull for image: ok=%v err=%v", ok, err)
	}
	if !reg.grants(tok, name, "pull") || reg.grants(tok, name, "push") || reg.grants(tok, "levelrail/other", "pull") {
		t.Fatal("pull token must grant pull on its repository only")
	}
	req, _ := http.NewRequest(http.MethodGet, regSrv.URL+"/v2/"+name+"/manifests/"+want, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	gs := sha256.Sum256(got)
	if resp.StatusCode != http.StatusOK || "sha256:"+hex.EncodeToString(gs[:]) != want {
		t.Fatalf("pull by digest: status %d, content digest mismatch", resp.StatusCode)
	}

	// Job end: the revoked token gets the bare 401 a builder reads as an auth failure, which selects tar.
	issuer.Revoke(job)
	if st, _ := pushThroughChallenge(t, regSrv.URL, name, "b2", job, manifest); st != http.StatusUnauthorized {
		t.Fatalf("a revoked job token must be refused, got %d", st)
	}
}
