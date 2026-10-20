package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerssh"
	"github.com/GLINCKER/levelrail/internal/imagemove"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const sshTestKey = "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret-key-material\n-----END OPENSSH PRIVATE KEY-----"

const sshTestFingerprint = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type fakeReader struct{ data string }

func (f fakeReader) InspectAll(context.Context) ([]byte, error) { return []byte(f.data), nil }

type tunnelCapture struct {
	mu  sync.Mutex
	cfg dockerssh.Config
}

func newSSHHarness(t *testing.T, saver *moveSaver) (*appImportHarness, *fakeImageRuntime, *tunnelCapture) {
	t.Helper()
	db := openTestDB(t)
	fake := &fakeImageRuntime{images: map[string]string{}, got: map[string]int{},
		ids: map[string]string{moveRefWeb: moveIDWeb, moveRefAPI: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}}
	rt := NewRouter(nil, testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	capture := &tunnelCapture{}
	rt.appImportLive.newInspectReader = func(dockerssh.Config) platformimport.InspectReader { return fakeReader{data: moveSnapshot()} }
	rt.appImportLive.newTunnelSaver = func(cfg dockerssh.Config) imagemove.Saver {
		capture.mu.Lock()
		capture.cfg = cfg
		capture.mu.Unlock()
		return saver
	}
	h := &appImportHarness{t: t, rt: rt, db: db, cookie: loginTestSession(t, rt, db)}
	return h, fake, capture
}

func sshBody() string {
	return `{"platform":"docker-ssh","ssh":{"host":"127.0.0.1","port":2200,"user":"root","private_key":` + jsonString(sshTestKey) +
		`,"host_key_fingerprint":"` + sshTestFingerprint + `"}}`
}

func TestSecretBytesNeverPrintOrMarshal(t *testing.T) {
	c := sshConnection{Host: "h", User: "u", PrivateKey: secretBytes("hunter2"), Password: secretBytes("pw")}
	raw, err := json.Marshal(c) //nolint:gosec // proves the secret fields marshal redacted
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{string(raw), fmt.Sprint(c.PrivateKey), fmt.Sprintf("%v %+v %#v", c, c, c.Password)} {
		if strings.Contains(out, "hunter2") || strings.Contains(out, `"pw"`) {
			t.Fatalf("secret leaked: %s", out)
		}
	}
	var back sshConnection
	if err := json.Unmarshal([]byte(`{"host":"h","user":"u","private_key":"abc"}`), &back); err != nil || string(back.PrivateKey) != "abc" {
		t.Fatalf("decode = %q, %v", back.PrivateKey, err)
	}
	back.wipe()
	if string(back.PrivateKey) == "abc" {
		t.Fatal("wipe left the key")
	}
}

func TestDockerSSHSessionRequiresFingerprintAndNeverEchoesKey(t *testing.T) {
	h, _, _ := newSSHHarness(t, &moveSaver{})
	noFP := `{"platform":"docker-ssh","ssh":{"host":"127.0.0.1","user":"root","private_key":` + jsonString(sshTestKey) + `}}`
	rec := h.call(http.MethodPost, appImportBase+"/sessions", noFP)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "host_key_fingerprint") {
		t.Fatalf("no fingerprint: %d %s", rec.Code, rec.Body.String())
	}
	bad := `{"platform":"docker-ssh","ssh":{"host":"bad host!","user":"root","private_key":"k","host_key_fingerprint":"x"}}`
	if rec = h.call(http.MethodPost, appImportBase+"/sessions", bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad host: %d %s", rec.Code, rec.Body.String())
	}
	if rec = h.call(http.MethodPost, appImportBase+"/sessions", `{"platform":"docker-ssh"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no ssh: %d", rec.Code)
	}
	rec = h.call(http.MethodPost, appImportBase+"/sessions", sshBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	for _, leak := range []string{"secret-key-material", sshTestFingerprint, "private_key"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("the session view contains %q", leak)
		}
	}
	v := h.view(rec)
	if v.Platform != "docker-ssh" || v.SourceURL != "ssh://127.0.0.1:2200" || !v.Connected || len(v.Items) == 0 {
		t.Fatalf("view = platform %s url %s connected %v items %d", v.Platform, v.SourceURL, v.Connected, len(v.Items))
	}
	if len(v.Notes) != 0 {
		t.Fatalf("a pinned session carries warnings: %v", v.Notes)
	}
	get := h.call(http.MethodGet, appImportBase+"/sessions/"+v.ID, "")
	if strings.Contains(get.Body.String(), "secret-key-material") {
		t.Fatal("GET session echoes the key")
	}
}

func TestDockerSSHInsecureHostKeyIsWarnedInTheView(t *testing.T) {
	h, _, _ := newSSHHarness(t, &moveSaver{})
	body := `{"platform":"docker-ssh","ssh":{"host":"127.0.0.1","user":"root","private_key":"k","insecure_host_key":true}}`
	rec := h.call(http.MethodPost, appImportBase+"/sessions", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if v := h.view(rec); len(v.Notes) == 0 || !strings.Contains(v.Notes[0], "Host key verification is off") {
		t.Fatalf("notes = %v", v.Notes)
	}
}

func TestDockerSSHImagesComeThroughTheTunnel(t *testing.T) {
	saver := &moveSaver{}
	h, fake, capture := newSSHHarness(t, saver)
	rec := h.call(http.MethodPost, appImportBase+"/sessions", sshBody())
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	id := h.view(rec).ID
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("transfer: %d %s", rec.Code, rec.Body.String())
	}
	v := h.waitImages(id)
	if web := imageByApp(v, "web"); web.State != imageMoveVerified || !web.Verified {
		t.Fatalf("web = %+v", web)
	}
	if len(saver.saved()) != 2 || len(fake.images) != 2 {
		t.Fatalf("saved %v loaded %d", saver.saved(), len(fake.images))
	}
	capture.mu.Lock()
	cfg := capture.cfg
	capture.mu.Unlock()
	if cfg.Host != "127.0.0.1" || cfg.Port != 2200 || cfg.User != "root" || cfg.Fingerprint != sshTestFingerprint || !strings.Contains(string(cfg.PrivateKey), "BEGIN") && !allZero(cfg.PrivateKey) {
		t.Fatalf("tunnel config = %+v", cfg)
	}
	if rec = h.call(http.MethodDelete, appImportBase+"/sessions/"+id, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if !allZero(cfg.PrivateKey) {
		t.Fatal("the private key was not zeroed when the session was forgotten")
	}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func TestDockerSSHImageTransferNeedsAConnectedSession(t *testing.T) {
	h, _, _ := newSSHHarness(t, &moveSaver{})
	rec := h.call(http.MethodPost, appImportBase+"/sessions", sshBody())
	id := h.view(rec).ID
	h.rt.appImportLive.forget(id)
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "reconnect") {
		t.Fatalf("transfer without connection: %d %s", rec.Code, rec.Body.String())
	}
}

func TestDockerSSHVolumeCopyGuards(t *testing.T) {
	h, _, _ := newSSHHarness(t, &moveSaver{})
	rec := h.call(http.MethodPost, appImportBase+"/sessions", sshBody())
	id := h.view(rec).ID
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/volumes/copy", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("copy before staging: %d %s", rec.Code, rec.Body.String())
	}
	if rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/volumes/copy", ""); rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	other, _, _ := newImageMoveHarness(t, &moveSaver{})
	rec = other.call(http.MethodPost, appImportBase+"/sessions", `{"platform":"docker","snapshot":`+jsonString(moveSnapshot())+`}`)
	oid := other.view(rec).ID
	if rec = other.call(http.MethodPost, appImportBase+"/sessions/"+oid+"/volumes/copy", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("copy on a non docker-ssh session: %d", rec.Code)
	}
}

func TestSSHTestEndpointTypedFailures(t *testing.T) {
	h, _, _ := newSSHHarness(t, &moveSaver{})
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	realKey := string(pem.EncodeToMemory(blk))
	call := func(path, body string) sshProbeResult {
		rec := h.call(http.MethodPost, appImportBase+"/ssh/"+path, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "OPENSSH PRIVATE KEY") {
			t.Fatal("the probe echoes the key")
		}
		var res sshProbeResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	noFP := `{"ssh":{"host":"127.0.0.1","user":"root","private_key":` + jsonString(realKey) + `}}`
	if res := call("test", noFP); res.OK || res.Kind != dockerssh.KindHostKeyRequired {
		t.Fatalf("no fingerprint: %+v", res)
	}
	withFP := `{"ssh":{"host":"127.0.0.1","port":2200,"user":"root","private_key":` + jsonString(realKey) + `,"host_key_fingerprint":"` + sshTestFingerprint + `"}}`
	if res := call("test", withFP); res.OK || res.Kind != dockerssh.KindBlockedAddress {
		t.Fatalf("loopback without opt in: %+v", res)
	}
	if res := call("host-keys", `{"ssh":{"host":"169.254.169.254","user":"root"}}`); res.OK || res.Kind != dockerssh.KindBlockedAddress {
		t.Fatalf("metadata address: %+v", res)
	}
	if res := call("host-keys", `{"ssh":{"host":"127.0.0.1","user":"root"}}`); res.OK || res.Kind != dockerssh.KindBlockedAddress {
		t.Fatalf("host-keys loopback: %+v", res)
	}
	rec := h.call(http.MethodPost, appImportBase+"/ssh/test", `{"ssh":{"host":"bad host!","user":"root"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad target: %d", rec.Code)
	}
	rec = h.call(http.MethodPost, appImportBase+"/ssh/test", `{"allow_private":true,"ssh":{"host":"10.0.0.1","user":"root","private_key":"k"}}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), platformimport.AllowPrivateEnv) {
		t.Fatalf("private opt in without env: %d %s", rec.Code, rec.Body.String())
	}
}
