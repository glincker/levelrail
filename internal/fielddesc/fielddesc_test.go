package fielddesc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupTargetVisibilityAndValidation(t *testing.T) {
	r, _ := Get(BackupTarget)
	if got := r.Validate(map[string]string{"provider": "local"}); len(got) != 2 {
		t.Fatalf("local needs name and path, got %v", got)
	}
	key := map[string]string{"provider": "sftp", "auth": "key", "name": "n", "endpoint": "h", "bucket": "b", "access_key_id": "u", "host_key": "k"}
	problems := r.Validate(key)
	if len(problems) != 1 || problems[0].Key != "private_key" {
		t.Fatalf("sftp key auth needs private_key only, got %v", problems)
	}
	if got := r.Validate(map[string]string{"provider": "nope", "name": "n"}); len(got) == 0 {
		t.Fatal("unknown provider must fail")
	}
}

func TestMaskedHidesSecrets(t *testing.T) {
	r, _ := Get(BackupTarget)
	m := r.Masked(map[string]string{"secret_access_key": "abc", "name": "x", "private_key": ""})
	if m["secret_access_key"] != SecretMask || m["name"] != "x" || m["private_key"] != "" {
		t.Fatalf("unexpected mask result %v", m)
	}
}

func TestLogDrainTargetRequiredOnlyForHTTP(t *testing.T) {
	r, _ := Get(LogDrain)
	if got := r.Validate(map[string]string{"type": "syslog"}); len(got) != 0 {
		t.Fatalf("syslog target is optional, got %v", got)
	}
	got := r.Validate(map[string]string{"type": "http"})
	if len(got) != 1 || got[0].Message != "target is required" {
		t.Fatalf("http needs a target, got %v", got)
	}
	if got := r.Validate(map[string]string{"type": "pigeon"}); len(got) == 0 {
		t.Fatal("unknown type must fail")
	}
}

// A resource is complete only with locale keys in every language, a docs row, a flag
// or an explicit form only mark, and a non empty doc line. Adding a resource file is
// the whole job: this test then holds it to the same bar as the others.
func TestEveryDescriptorIsComplete(t *testing.T) {
	langs := []string{"en", "de", "fr", "ja", "es", "pt-BR", "zh-CN"}
	repo := filepath.Join("..", "..")
	locales := map[string]map[string]any{}
	load := func(lang, ns string) map[string]any {
		k := lang + "/" + ns
		if m, ok := locales[k]; ok {
			return m
		}
		raw, err := os.ReadFile(filepath.Join(repo, "web", "src", "locales", lang, ns+".json")) //nolint:gosec // fixed repo path
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		locales[k] = m
		return m
	}
	for _, name := range Names() {
		res, _ := Get(name)
		docs, err := os.ReadFile(filepath.Join(repo, "docs", res.Docs.File)) //nolint:gosec // fixed repo path
		if err != nil {
			t.Fatalf("%s: docs page missing, run go run ./scripts/gen-field-docs: %v", name, err)
		}
		if string(docs) != DocsPage(res) {
			t.Errorf("%s: docs page is stale, run go run ./scripts/gen-field-docs", name)
		}
		for _, f := range res.Fields {
			if f.Doc == "" {
				t.Errorf("%s.%s: empty doc", name, f.ID)
			}
			if f.Flag == "" && !f.FormOnly {
				flagged := false
				for _, o := range res.Fields {
					if o.Key == f.Key && o.Flag != "" {
						flagged = true
					}
				}
				if !flagged {
					t.Errorf("%s.%s: key %q has no CLI flag and is not FormOnly", name, f.ID, f.Key)
				}
			}
			for _, k := range []string{f.LabelKey, f.HelpKey} {
				ns, key, found := strings.Cut(k, ":")
				if !found {
					t.Errorf("%s.%s: key %q lacks a namespace", name, f.ID, k)
					continue
				}
				for _, l := range langs {
					var cur any = load(l, ns)
					for _, part := range strings.Split(key, ".") {
						m, ok := cur.(map[string]any)
						if !ok {
							cur = nil
							break
						}
						cur = m[part]
					}
					if s, ok := cur.(string); !ok || s == "" {
						t.Errorf("%s.%s: locale %s misses %s", name, f.ID, l, k)
					}
				}
			}
		}
	}
}

func TestNotificationChannelCredentialFieldsAreSecret(t *testing.T) {
	res, ok := Get(NotificationChannel)
	if !ok {
		t.Fatal("registry missing")
	}
	want := map[string]bool{"notify_url_webhook": true, "notify_url_gotify": true, "notify_url_ntfy": true, "pushover_api_token": true, "resend_api_key": true, "notify_url_email": false}
	for _, f := range res.Fields {
		if w, ok := want[f.ID]; ok && f.Secret != w {
			t.Errorf("%s secret = %v, want %v", f.ID, f.Secret, w)
		}
	}
}
