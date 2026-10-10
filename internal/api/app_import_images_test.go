package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/imagemove"
)

const (
	moveIDWeb  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	moveIDAPI  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	moveIDBad  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	moveRefWeb = "webwebwebwebwebwebwebweb:1111111111111111111111111111111111111111"
	moveRefAPI = "apiapiapiapiapiapiapiapi:2222222222222222222222222222222222222222"
	moveRefBad = "Bad;id:3333333333333333333333333333333333333333"
	moveKey    = "-----BEGIN OPENSSH PRIVATE KEY-----\nnot-a-real-key\n-----END OPENSSH PRIVATE KEY-----"
)

func moveSnapshot() string {
	c := func(id, name, ref, imageID string) string {
		return `{"Id":"` + id + `","Name":"/` + name + `","Image":"` + imageID + `","Created":"2026-01-01T00:00:00Z","Config":{"Image":"` + ref +
			`","ExposedPorts":{"3000/tcp":{}},"Labels":{}},"State":{"Running":true}}`
	}
	return "[" + strings.Join([]string{
		c("c1", "web", moveRefWeb, moveIDWeb),
		c("c2", "api", moveRefAPI, moveIDAPI),
		c("c3", "bad", moveRefBad, moveIDBad),
		c("c4", "proxy", "nginx:1.27", "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
	}, ",") + "]"
}

type fakeImageRuntime struct {
	docker.Runtime
	mu     sync.Mutex
	images map[string]string
	// ids is what each ref resolves to after a load.
	ids map[string]string
	got map[string]int
}

func (f *fakeImageRuntime) LoadImage(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	ref := string(b)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images[ref] = f.ids[ref]
	f.got[ref] = len(b)
	return nil
}

func (f *fakeImageRuntime) InspectImageID(_ context.Context, ref string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.images[ref], nil
}

// moveSaver "streams" the ref itself, so the fake runtime knows what loaded.
type moveSaver struct {
	mu     sync.Mutex
	refs   []string
	block  bool
	target imagemove.Target
	creds  imagemove.Credentials
}

func (s *moveSaver) Save(ctx context.Context, ref string) (io.ReadCloser, error) {
	s.mu.Lock()
	s.refs = append(s.refs, ref)
	s.mu.Unlock()
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return io.NopCloser(bytes.NewReader([]byte(ref))), nil
}

func (s *moveSaver) saved() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.refs...)
}

func newImageMoveHarness(t *testing.T, saver *moveSaver) (*appImportHarness, *fakeImageRuntime, string) {
	t.Helper()
	db := openTestDB(t)
	fake := &fakeImageRuntime{images: map[string]string{}, got: map[string]int{},
		ids: map[string]string{moveRefWeb: moveIDWeb, moveRefAPI: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}}
	rt := NewRouter(nil, testBrand(), db, WithExecRuntime(func(string) (docker.Runtime, error) { return fake, nil }))
	rt.appImportLive.newSaver = func(tg imagemove.Target, c imagemove.Credentials, _ *imagemove.HostKeys) imagemove.Saver {
		saver.target, saver.creds = tg, c
		return saver
	}
	h := &appImportHarness{t: t, rt: rt, db: db, cookie: loginTestSession(t, rt, db)}
	body, _ := json.Marshal(map[string]string{"platform": "docker", "snapshot": moveSnapshot()})
	rec := h.call(http.MethodPost, appImportBase+"/sessions", string(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	return h, fake, h.view(rec).ID
}

func (h *appImportHarness) images(rec *httptest.ResponseRecorder) appImportImagesResource {
	h.t.Helper()
	var v appImportImagesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		h.t.Fatalf("decode images: %v: %s", err, rec.Body.String())
	}
	return v
}

func (h *appImportHarness) waitImages(id string) appImportImagesResource {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		v := h.images(h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/images/status", ""))
		if !v.Running {
			return v
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("image move still running: %+v", v)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func imageByApp(v appImportImagesResource, app string) appImportImage {
	for _, img := range v.Images {
		if img.App == app {
			return img
		}
	}
	return appImportImage{}
}

func TestAppImportImagesListOnlyHostBuilt(t *testing.T) {
	h, _, id := newImageMoveHarness(t, &moveSaver{})
	v := h.images(h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/images", ""))
	if len(v.Images) != 3 || !v.Supported || v.Running || v.CredentialsHeld {
		t.Fatalf("images = %+v", v)
	}
	if web := imageByApp(v, "web"); web.Image != moveRefWeb || web.SourceImageID != moveIDWeb || web.State != imageMovePending {
		t.Fatalf("web = %+v", web)
	}
	if imageByApp(v, "proxy").Image != "" {
		t.Fatal("a registry image was listed")
	}
}

func TestAppImportImagesTransfer(t *testing.T) {
	saver := &moveSaver{}
	h, fake, id := newImageMoveHarness(t, saver)
	rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{"ssh":"root@old.example.com:2222","private_key":`+jsonString(moveKey)+`}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("transfer: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "not-a-real-key") {
		t.Fatal("response echoes the private key")
	}
	v := h.waitImages(id)
	if web := imageByApp(v, "web"); web.State != imageMoveVerified || !web.Verified || web.Bytes != int64(len(moveRefWeb)) || web.LoadedImageID != moveIDWeb {
		t.Fatalf("web = %+v", web)
	}
	if api := imageByApp(v, "api"); api.State != imageMoveFailed || !strings.Contains(api.Error, "has ID") {
		t.Fatalf("api = %+v, want an ID mismatch failure", api)
	}
	if bad := imageByApp(v, "bad"); bad.State != imageMoveFailed || !strings.Contains(bad.Error, "not allowed") {
		t.Fatalf("bad = %+v, want a validation failure", bad)
	}
	for _, ref := range saver.saved() {
		if ref == moveRefBad {
			t.Fatal("an invalid ref reached the source host")
		}
	}
	if saver.target.String() != "root@old.example.com:2222" || string(saver.creds.PrivateKey) != moveKey {
		t.Fatalf("saver got %v", saver.target)
	}
	if v.Source != "root@old.example.com:2222" || !v.CredentialsHeld {
		t.Fatalf("view = %+v", v)
	}
	if fake.got[moveRefWeb] == 0 {
		t.Fatal("web was not loaded into the target runtime")
	}

	sess, err := h.db.GetAppImportSession(context.Background(), id)
	if err != nil || sess.Step != appImportStepImages {
		t.Fatalf("step = %q, %v", sess.Step, err)
	}

	// A retry reuses the held key and skips the image already verified.
	before := len(saver.saved())
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{"items":["web"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
	}
	v = h.waitImages(id)
	if len(saver.saved()) != before || imageByApp(v, "web").State != imageMoveVerified {
		t.Fatalf("retry re-streamed a verified image: %v", saver.saved())
	}

	// After a restart the list re-derives verified state from the target.
	h.rt.appImportLive.forgetMove(id)
	v = h.images(h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/images", ""))
	if web := imageByApp(v, "web"); web.State != imageMoveVerified {
		t.Fatalf("after restart web = %+v", web)
	}
}

func TestAppImportImagesTransferValidation(t *testing.T) {
	h, _, id := newImageMoveHarness(t, &moveSaver{})
	cases := []struct{ name, body, want string }{
		{"no target", `{"private_key":"k"}`, "ssh is required"},
		{"option injection", `{"ssh":"-oProxyCommand=sh@host","private_key":"k"}`, "not allowed"},
		{"host injection", `{"ssh":"root@host;id","private_key":"k"}`, "not a hostname"},
		{"no key", `{"ssh":"root@host"}`, "private key"},
		{"port conflict", `{"ssh":"root@host:22","port":2222,"private_key":"k"}`, "conflicts"},
		{"nothing selected", `{"ssh":"root@host","private_key":"k","items":["proxy"]}`, "no selected app"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", c.body)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("got %d %s, want 400 with %q", rec.Code, rec.Body.String(), c.want)
			}
		})
	}
}

func TestAppImportImagesCancel(t *testing.T) {
	saver := &moveSaver{block: true}
	h, _, id := newImageMoveHarness(t, saver)
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/cancel", ""); rec.Code != http.StatusConflict {
		t.Fatalf("cancel idle: %d", rec.Code)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{"ssh":"root@host","use_agent":true}`); rec.Code != http.StatusAccepted {
		t.Fatalf("transfer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{"ssh":"root@host","use_agent":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("second transfer: %d", rec.Code)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/cancel", ""); rec.Code != http.StatusAccepted {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	v := h.waitImages(id)
	for _, img := range v.Images {
		if img.State != imageMoveCancelled && img.State != imageMoveFailed {
			t.Fatalf("%s state = %q after cancel", img.App, img.State)
		}
	}
	if imageByApp(v, "web").State != imageMoveCancelled {
		t.Fatalf("web = %+v", imageByApp(v, "web"))
	}
}

func TestAppImportImagesNoRuntime(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db)
	h := &appImportHarness{t: t, rt: rt, db: db, cookie: loginTestSession(t, rt, db)}
	body, _ := json.Marshal(map[string]string{"platform": "docker", "snapshot": moveSnapshot()})
	id := h.view(h.call(http.MethodPost, appImportBase+"/sessions", string(body))).ID
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/images/transfer", `{"ssh":"root@host","private_key":"k"}`); rec.Code != http.StatusNotImplemented {
		t.Fatalf("transfer = %d", rec.Code)
	}
	if v := h.images(h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/images", "")); v.Supported {
		t.Fatal("supported without a runtime")
	}
}

func TestImageRuntimeRejectsRemoteWithoutLoad(t *testing.T) {
	rt := NewRouter(nil, testBrand(), openTestDB(t), WithExecRuntime(func(string) (docker.Runtime, error) { return &fakeExecAppRuntime{}, nil }))
	if _, err := rt.imageRuntime("node-2"); err == nil || !strings.Contains(err.Error(), "cannot load images") {
		t.Fatalf("err = %v", err)
	}
	rt = NewRouter(nil, testBrand(), openTestDB(t), WithExecRuntime(func(string) (docker.Runtime, error) { return nil, errors.New("offline") }))
	if _, err := rt.imageRuntime("node-2"); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("err = %v", err)
	}
}
