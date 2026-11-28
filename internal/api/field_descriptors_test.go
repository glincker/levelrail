package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/fielddesc"
)

// A key the registry marks secret must never appear in the read shape.
func TestBackupTargetResourceCarriesNoDescriptorSecret(t *testing.T) {
	res, _ := fielddesc.Get(fielddesc.BackupTarget)
	secrets := map[string]bool{}
	for _, f := range res.Fields {
		if f.Secret {
			secrets[f.Key] = true
		}
	}
	rt := reflect.TypeOf(backupTargetResource{})
	for i := 0; i < rt.NumField(); i++ {
		name, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		if secrets[name] {
			t.Errorf("backupTargetResource echoes secret %q", name)
		}
	}
}

func TestFieldDescriptorsServed(t *testing.T) {
	rt, db := newTestRouterWithBackupSecrets(t, &fakeBackupSecretsSetter{})
	cookie := loginTestSession(t, rt, db)
	rec := apiDo(t, rt, cookie, "GET", "/api/v1/backup-targets/field-descriptors", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var got fielddesc.Resource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Fields) == 0 {
		t.Fatalf("bad body %v", err)
	}
}

// A blank credential on update keeps the stored one, and no response echoes a secret.
func TestBackupTargetUpdateKeepsSecretsAndNeverEchoes(t *testing.T) {
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	const secret = "s3cr3t-value-xyz"
	create := apiDo(t, rt, cookie, "POST", "/api/v1/backup-targets", `{"name":"b","provider":"aws","bucket":"bk","access_key_id":"AKIDTEST","secret_access_key":"`+secret+`"}`)
	if create.Code != 201 && create.Code != 200 {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}
	var created backupTargetResource
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	before := len(setter.calls)
	upd := apiDo(t, rt, cookie, "PUT", "/api/v1/backup-targets/"+created.ID, `{"name":"renamed","provider":"aws","bucket":"bk"}`)
	if upd.Code != 200 {
		t.Fatalf("update status %d: %s", upd.Code, upd.Body.String())
	}
	if len(setter.calls) != before {
		t.Fatalf("blank credentials rewrote %d secrets", len(setter.calls)-before)
	}
	for _, body := range []string{create.Body.String(), upd.Body.String(), apiDo(t, rt, cookie, "GET", "/api/v1/backup-targets/"+created.ID, "").Body.String(), apiDo(t, rt, cookie, "GET", "/api/v1/backup-targets", "").Body.String()} {
		if strings.Contains(body, secret) || strings.Contains(body, "AKIDTEST") {
			t.Fatalf("response echoes a credential: %s", body)
		}
	}
}

func TestLogDrainFieldDescriptorsServed(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := apiDo(t, rt, cookie, "GET", "/api/v1/log-drains/field-descriptors", "")
	var got fielddesc.Resource
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Name != fielddesc.LogDrain {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestGenericFieldDescriptorsServeEveryResource(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, name := range fielddesc.Names() {
		rec := apiDo(t, rt, cookie, "GET", "/api/v1/field-descriptors/"+name, "")
		var got fielddesc.Resource
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Name != name || len(got.Fields) == 0 {
			t.Fatalf("%s: status %d body %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := apiDo(t, rt, cookie, "GET", "/api/v1/field-descriptors/nope", ""); rec.Code != 404 {
		t.Fatalf("unknown resource status %d", rec.Code)
	}
}
