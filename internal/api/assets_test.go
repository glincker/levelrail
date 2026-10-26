package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newAssetEnv(t *testing.T) (*storageEnv, storageDestinationResource) {
	t.Helper()
	e := newStorageEnv(t)
	d := e.createDestination(t, "")
	secrets := &memSecrets{}
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "access_key_id", "AKIDSECRET")
	_ = secrets.SetValue(context.Background(), store.BackupTargetSecretsKey(d.ID), "secret_access_key", "topsecretvalue")
	resolver := &objectstore.Resolver{Store: e.db, Secrets: secrets, MaxAttempts: 1}
	e.rt.SetStorage(StorageDeps{Options: e.db, Clients: resolver, Buckets: e.db, AssetBuckets: e.db})
	return e, d
}

func (e *storageEnv) createAssetBucket(t *testing.T, targetID, extra string) assetBucketResource {
	t.Helper()
	rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"media","target_id":"`+targetID+`"`+extra+`}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create asset bucket = %d: %s", rec.Code, rec.Body.String())
	}
	var b assetBucketResource
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeAssetBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestAssetBucketLifecycleAndExposureAudit(t *testing.T) {
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, `,"quota_bytes":1000`)
	if b.Visibility != store.AssetVisibilityPrivate || b.ServingMode != store.AssetServingDirect || b.Preset != objectstore.PresetMinIO || b.QuotaBytes != 1000 {
		t.Fatalf("defaults wrong: %+v", b)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"media","target_id":"`+d.ID+`"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"x2","target_id":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown target = %d, want 400", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"px","target_id":"`+d.ID+`","prefix":"a/","serving_mode":"proxied"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("proxied = %d, want 400", rec.Code)
	}

	path := "/api/v1/assets/buckets/" + b.ID
	if rec := e.do(t, http.MethodPatch, path, `{"visibility":"public"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), errCodeAssetConfirm) {
		t.Fatalf("public without confirm = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPatch, path, `{"visibility":"public","confirm_name":"other"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("public with wrong confirm = %d", rec.Code)
	}
	rec := e.do(t, http.MethodPatch, path, `{"visibility":"public","confirm_name":"media","public_base_url":"https://assets.example.com/","referrer_allow":["App.Example.com"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("public with confirm = %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeAssetBody[assetBucketResource](t, rec)
	if got.Visibility != store.AssetVisibilityPublic || got.PublicBaseURL != "https://assets.example.com" || !got.RefuseNoReferer || got.ReferrerAllow[0] != "app.example.com" {
		t.Fatalf("updated = %+v", got)
	}
	entries, err := e.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: auditAssetExposureChanged})
	if err != nil || len(entries) != 1 {
		t.Fatalf("exposure audit entries = %d, %v", len(entries), err)
	}
	if !strings.Contains(entries[0].Detail, `"new_visibility":"public"`) || !strings.Contains(entries[0].Detail, `"old_visibility":"private"`) {
		t.Fatalf("audit detail = %s", entries[0].Detail)
	}
	if rec := e.do(t, http.MethodPatch, path, `{"quota_bytes":5}`); rec.Code != http.StatusOK {
		t.Fatalf("quota patch = %d", rec.Code)
	}
	if n, _ := e.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: auditAssetExposureChanged}); len(n) != 1 {
		t.Fatalf("a quota change must not write an exposure audit entry, have %d", len(n))
	}

	list := e.do(t, http.MethodGet, "/api/v1/assets/buckets", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "AKIDSECRET") || strings.Contains(list.Body.String(), "topsecretvalue") {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	if buckets := decodeAssetBody[[]assetBucketResource](t, list); len(buckets) != 1 || buckets[0].QuotaBytes != 5 {
		t.Fatalf("list = %+v", buckets)
	}

	if rec := e.do(t, http.MethodGet, "/api/v1/storage/buckets/"+b.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("asset bucket on the app bucket route = %d, want 404", rec.Code)
	}
	if rec := e.do(t, http.MethodDelete, "/api/v1/storage/buckets/"+b.ID+"?purge=true", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("purge via app bucket route = %d, want 404", rec.Code)
	}
	if rec := e.do(t, http.MethodGet, publicBucketPathPrefix+b.ID+"/a.txt", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("public bucket route served an asset bucket: %d", rec.Code)
	}

	e.srv.Put("keep.txt", []byte("x"), time.Now())
	if rec := e.do(t, http.MethodDelete, path, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if _, ok := e.srv.Data("keep.txt"); !ok {
		t.Fatal("deleting the connection must not delete provider data")
	}
	if rec := e.do(t, http.MethodGet, path, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d", rec.Code)
	}
}

func TestAssetPresignedUploadConfirmAndDelete(t *testing.T) {
	t.Setenv(envAssetsUploadMaxBytes, "2048")
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, `,"prefix":"site"`)
	base := "/api/v1/assets/buckets/" + b.ID

	if rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"a.png","content_type":"image/png","size":4096}`); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize = %d, want 413", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"a.png","content_type":"image/png"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing size = %d, want 400", rec.Code)
	}
	if rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"../a.png","size":10}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad key = %d, want 400", rec.Code)
	}

	rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"img/a.png","content_type":"image/png","size":512}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("presign = %d: %s", rec.Code, rec.Body.String())
	}
	up := decodeAssetBody[assetUploadResource](t, rec)
	if up.Method != http.MethodPut || up.Headers["Content-Type"] != "image/png" || !strings.Contains(up.URL, "content-length") {
		t.Fatalf("upload contract = %+v", up)
	}
	if strings.Contains(up.URL, "topsecretvalue") {
		t.Fatal("presigned URL leaked the secret key")
	}

	if rec := e.do(t, http.MethodPost, base+"/uploads/confirm", `{"key":"img/a.png"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("confirm before upload = %d, want 404", rec.Code)
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, up.URL, bytes.NewReader(make([]byte, 512)))
	req.Header.Set("Content-Type", "image/png")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("browser PUT: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	if _, ok := e.srv.Data("site/img/a.png"); !ok {
		t.Fatalf("object not stored under the prefix: %v", e.srv.Keys())
	}

	rec = e.do(t, http.MethodPost, base+"/uploads/confirm", `{"key":"img/a.png"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}
	if a := decodeAssetBody[assetResource](t, rec); a.Size != 512 || a.Key != "img/a.png" || !strings.HasPrefix(a.Uploader, "user:") {
		t.Fatalf("confirmed asset = %+v", a)
	}

	direct := httptest.NewRecorder()
	dreq := authedRequest(t, e.cookie, http.MethodPut, base+"/objects?key=notes.txt", "hello")
	dreq.Header.Set("Content-Type", "text/plain")
	e.rt.Handler().ServeHTTP(direct, dreq)
	if direct.Code != http.StatusCreated {
		t.Fatalf("direct upload = %d: %s", direct.Code, direct.Body.String())
	}
	big := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(big, authedRequest(t, e.cookie, http.MethodPut, base+"/objects?key=big.bin", strings.Repeat("x", 3000)))
	if big.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize direct upload = %d, want 413", big.Code)
	}

	list := e.do(t, http.MethodGet, base+"/assets?limit=1", "")
	page := decodeAssetBody[assetListResource](t, list)
	if list.Code != http.StatusOK || len(page.Assets) != 1 || page.Cursor == "" || page.Assets[0].Key != "img/a.png" || page.Assets[0].Uploader == "" {
		t.Fatalf("first page = %d %+v", list.Code, page)
	}
	next := decodeAssetBody[assetListResource](t, e.do(t, http.MethodGet, base+"/assets?limit=1&cursor="+page.Cursor, ""))
	if len(next.Assets) != 1 || next.Assets[0].Key != "notes.txt" || next.Cursor != "" {
		t.Fatalf("second page = %+v", next)
	}

	if rec := e.do(t, http.MethodDelete, base+"/objects?key=img/a.png", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if _, ok := e.srv.Data("site/img/a.png"); ok {
		t.Fatal("object still at the provider")
	}
	if _, err := e.db.GetAsset(context.Background(), b.ID, "img/a.png"); err == nil {
		t.Fatal("index row survived delete")
	}
}

func TestAssetUploadQuotaAndTTL(t *testing.T) {
	t.Setenv(envAssetsPresignMaxTTL, "2m")
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, `,"quota_bytes":100`)
	e.srv.Put("buckets/old", make([]byte, 90), time.Now())
	base := "/api/v1/assets/buckets/" + b.ID
	// Empty prefix counts the whole provider bucket, so 90 existing bytes leave no room for 20.
	if rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"a","content_type":"text/plain","size":20}`); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over quota = %d, want 413", rec.Code)
	}
	rec := e.do(t, http.MethodPost, base+"/uploads", `{"key":"a","content_type":"text/plain","size":5,"expires_seconds":99999}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("within quota = %d: %s", rec.Code, rec.Body.String())
	}
	up := decodeAssetBody[assetUploadResource](t, rec)
	exp, err := time.Parse(time.RFC3339, up.ExpiresAt)
	if err != nil || time.Until(exp) > 2*time.Minute+5*time.Second {
		t.Fatalf("ttl not capped by env: %s (%v)", up.ExpiresAt, err)
	}
}

func (e *storageEnv) createAssetBucket2(t *testing.T, targetID, name string) assetBucketResource {
	t.Helper()
	rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"`+name+`","target_id":"`+targetID+`","prefix":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s = %d: %s", name, rec.Code, rec.Body.String())
	}
	return decodeAssetBody[assetBucketResource](t, rec)
}

func TestAssetURLs(t *testing.T) {
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, `,"prefix":"site","public_base_url":"https://assets.example.com","cdn_base_url":"https://cdn.example.com"`)
	urls := "/api/v1/assets/buckets/" + b.ID + "/urls?key=" + "my%20img/a.png"

	priv := decodeAssetBody[assetURLsResource](t, e.do(t, http.MethodGet, urls, ""))
	if priv.OriginURL != "" || priv.CDNURL != "" || !strings.Contains(priv.PresignedURL, "X-Amz-Signature") {
		t.Fatalf("private bucket must only hand out a presigned URL: %+v", priv)
	}
	if rec := e.do(t, http.MethodPatch, "/api/v1/assets/buckets/"+b.ID, `{"visibility":"public","confirm_name":"media"}`); rec.Code != http.StatusOK {
		t.Fatalf("make public = %d", rec.Code)
	}
	pub := decodeAssetBody[assetURLsResource](t, e.do(t, http.MethodGet, urls, ""))
	if pub.OriginURL != "https://assets.example.com/site/my%20img/a.png" || pub.CDNURL != "https://cdn.example.com/site/my%20img/a.png" || pub.PresignedURL != "" {
		t.Fatalf("public urls = %+v", pub)
	}
	if rec := e.do(t, http.MethodGet, "/api/v1/assets/buckets/"+b.ID+"/urls?key=..%2Fx", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad key = %d", rec.Code)
	}
}

func TestAssetProbeCORSAndExposure(t *testing.T) {
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, `,"public_base_url":"`+e.srv.URL+`/logs"`)
	probe := "/api/v1/assets/buckets/" + b.ID + "/probe"
	const origin = "https://dash.example.com"

	res := decodeAssetBody[assetProbeResource](t, e.do(t, http.MethodPost, probe, `{"origin":"`+origin+`"}`))
	if !res.OK || res.CORS == nil || res.CORS.Status != objectstore.CORSMissing || res.CORS.Rule.AllowedOrigins[0] != origin {
		t.Fatalf("cors check = %+v", res.CORS)
	}
	if res.Exposure == nil || res.Exposure.Public || len(res.Warnings) != 1 || res.Warnings[0].Code != warnCORSMissing {
		t.Fatalf("exposure/warnings = %+v %+v", res.Exposure, res.Warnings)
	}

	res = decodeAssetBody[assetProbeResource](t, e.do(t, http.MethodPost, probe, `{"origin":"`+origin+`","fix_cors":true}`))
	if res.CORS.Status != objectstore.CORSApplied || len(res.Warnings) != 0 {
		t.Fatalf("fix = %+v warnings %+v", res.CORS, res.Warnings)
	}

	e.srv.PublicRead = true
	res = decodeAssetBody[assetProbeResource](t, e.do(t, http.MethodPost, probe, `{"origin":"`+origin+`"}`))
	if res.Exposure == nil || !res.Exposure.Public || len(res.Warnings) != 1 || res.Warnings[0].Code != warnExposurePrivateButPublic {
		t.Fatalf("private bucket that answers anonymously must warn: %+v %+v", res.Exposure, res.Warnings)
	}
	if len(e.srv.Keys()) != 0 {
		t.Fatalf("probe left objects: %v", e.srv.Keys())
	}

	e.srv.DenyCORSWrite = true
	e.srv.PublicRead = false
	if rec := e.do(t, http.MethodPatch, "/api/v1/assets/buckets/"+b.ID, `{"visibility":"public","confirm_name":"media"}`); rec.Code != http.StatusOK {
		t.Fatalf("make public = %d", rec.Code)
	}
	res = decodeAssetBody[assetProbeResource](t, e.do(t, http.MethodPost, probe, `{"origin":"https://other.example.com","fix_cors":true}`))
	if res.CORS.Status != objectstore.CORSManual || res.CORS.Rule.AllowedOrigins[0] != "https://other.example.com" {
		t.Fatalf("manual fix = %+v", res.CORS)
	}
	codes := map[string]bool{}
	for _, w := range res.Warnings {
		codes[w.Code] = true
	}
	if !codes[warnCORSMissing] || !codes[warnExposurePublicButClosed] {
		t.Fatalf("warnings = %+v", res.Warnings)
	}

	e.srv.RejectAuth = true
	bad := decodeAssetBody[assetProbeResource](t, e.do(t, http.MethodPost, probe, `{}`))
	if bad.OK || bad.Reason != objectstore.ReasonInvalidCredentials || strings.Contains(bad.Message, "topsecretvalue") {
		t.Fatalf("bad credentials probe = %+v", bad)
	}
}

func TestAssetUsageAndCost(t *testing.T) {
	t.Setenv(envAssetsUsageMaxObjects, "2")
	e, d := newAssetEnv(t)
	b := e.createAssetBucket(t, d.ID, "")
	for _, k := range []string{"a", "b", "c"} {
		e.srv.Put(k, make([]byte, 1000), time.Now())
	}
	rec := e.do(t, http.MethodGet, "/api/v1/assets/buckets/"+b.ID+"/usage?egress_gb=50", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("usage = %d: %s", rec.Code, rec.Body.String())
	}
	u := decodeAssetBody[assetUsageResource](t, rec)
	if u.Objects != 2 || !u.AtLeast || u.Bytes != 2000 || u.TrafficMeasured || !u.Cost.Estimated || u.Cost.PriceTableDate == "" || u.Cost.EgressGB != 50 {
		t.Fatalf("usage = %+v", u)
	}
	if len(u.Cost.Compare) != 2 || u.Cost.Compare[0].Preset != objectstore.PresetAWS || u.Cost.Compare[0].EgressUSD <= 0 {
		t.Fatalf("comparison = %+v", u.Cost.Compare)
	}
	if rec := e.do(t, http.MethodGet, "/api/v1/assets/buckets/"+b.ID+"/usage?egress_gb=-1", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("negative egress = %d", rec.Code)
	}
}

func TestAssetBucketIAMProjectScope(t *testing.T) {
	e, d := newAssetEnv(t)
	ctx := context.Background()
	if err := e.db.SaveProject(ctx, store.Project{ID: "proj1", Name: "Site", CreatedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	inProject := e.createAssetBucket(t, d.ID, `,"project_id":"proj1","prefix":"p"`)
	other := e.createAssetBucket2(t, d.ID, "other")

	scoped := seedMatrixToken(t, e.db, "scoped", []string{AbilityRead})
	attachTestPolicy(t, e.db, "proj-write", "Allow", AbilityWrite, "project:proj1", store.PrincipalTypeToken, "scoped")
	call := func(method, path, tok string) int {
		return matrixDo(e.rt, method, path, bearer(tok))
	}
	up := "/api/v1/assets/buckets/" + inProject.ID + "/uploads"
	if got := call(http.MethodPost, up, scoped); got == http.StatusForbidden || got == http.StatusUnauthorized {
		t.Fatalf("project Allow should grant write on its bucket, got %d", got)
	}
	if got := call(http.MethodPost, "/api/v1/assets/buckets/"+other.ID+"/uploads", scoped); got != http.StatusForbidden {
		t.Fatalf("write outside the project = %d, want 403", got)
	}

	attachTestPolicy(t, e.db, "bucket-deny", "Deny", "*", "asset_bucket:"+inProject.ID, store.PrincipalTypeToken, "scoped")
	if got := call(http.MethodGet, "/api/v1/assets/buckets/"+inProject.ID, scoped); got != http.StatusForbidden {
		t.Fatalf("bucket Deny = %d, want 403", got)
	}
	list := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(list, bearerRequest(http.MethodGet, "/api/v1/assets/buckets", "", scoped))
	if buckets := decodeAssetBody[[]assetBucketResource](t, list); len(buckets) != 1 || buckets[0].ID != other.ID {
		t.Fatalf("a denied bucket must be hidden from the list: %+v", buckets)
	}

	readOnly := seedMatrixToken(t, e.db, "ro", []string{AbilityRead})
	if got := call(http.MethodPost, "/api/v1/assets/buckets", readOnly); got != http.StatusForbidden {
		t.Fatalf("create with read token = %d, want 403", got)
	}
	if rec := e.do(t, http.MethodPost, "/api/v1/assets/buckets", `{"name":"nope","target_id":"`+d.ID+`","project_id":"missing"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown project = %d, want 400", rec.Code)
	}
}
