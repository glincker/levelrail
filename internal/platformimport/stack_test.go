package platformimport

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func loadDokployCompose(t *testing.T, name string) dokployCompose {
	t.Helper()
	raw, err := os.ReadFile("testdata/dokploy/" + name) //nolint:gosec // fixed test fixture names
	if err != nil {
		t.Fatal(err)
	}
	var c dokployCompose
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDokployComposeStackReady(t *testing.T) {
	c := mapDokployCompose("shop", loadDokployCompose(t, "compose_stack.json"))
	if c.Verdict != ComposeReady || len(c.Blocked) != 0 {
		t.Fatalf("verdict %q blocked %v", c.Verdict, c.Blocked)
	}
	if !slices.Equal(c.Volumes, []string{"pgdata"}) {
		t.Fatalf("volumes %v", c.Volumes)
	}
	if !slices.Equal(c.Domains, []string{"shop.example.test", "admin.example.test"}) {
		t.Fatalf("domains %v", c.Domains)
	}
	plain, secrets := c.SplitEnv()
	if plain["APP_MODE"] != "production" || len(plain) != 1 {
		t.Fatalf("plain %v", plain)
	}
	if _, ok := secrets["DB_PASSWORD"]; !ok || len(secrets) != 1 {
		t.Fatalf("secret keys %d", len(secrets))
	}
	out, _ := json.Marshal(c)
	if bytes.Contains(out, []byte("fake-not-a-real-password")) || bytes.Contains(out, []byte("services:")) {
		t.Fatal("serialized compose app leaks the env or compose text")
	}
	p := BuildPlan(&Discovery{Platform: Dokploy, ComposeApps: []ComposeApp{c}}, PlanOptions{})
	if it := findItem(t, p.Report, "compose", "cmp-1"); it.Status != StatusMapped || it.Target != "shop-stack" {
		t.Fatalf("plan item %+v", it)
	}
}

func TestDokployComposeStackBlocked(t *testing.T) {
	c := mapDokployCompose("ops", loadDokployCompose(t, "compose_blocked.json"))
	joined := strings.Join(c.Blocked, "\n")
	if c.Verdict != ComposeBlocked || !strings.Contains(joined, "privileged") || !strings.Contains(joined, "Docker socket") {
		t.Fatalf("verdict %q blocked %v", c.Verdict, c.Blocked)
	}
	p := BuildPlan(&Discovery{Platform: Dokploy, ComposeApps: []ComposeApp{c}}, PlanOptions{})
	if it := findItem(t, p.Report, "compose", "cmp-2"); it.Status != StatusUnsupported || len(it.Reasons) < 2 {
		t.Fatalf("plan item %+v", it)
	}
}

func TestNewAppFileKeepsBytesAndRefusesTraversal(t *testing.T) {
	body := []byte("a = 1\r\n\tb = 2\n")
	f, err := NewAppFile("/etc/app/app.conf", body)
	if err != nil || !bytes.Equal(f.Content, body) || f.Secret {
		t.Fatalf("file %+v err %v", f, err)
	}
	if f, err := NewAppFile("/run/app/token.txt", []byte("x")); err != nil || !f.Secret {
		t.Fatalf("secret name not flagged: %+v %v", f, err)
	}
	for _, p := range []string{"etc/app.conf", "/etc/../root/.ssh/keys", "/etc\\app.conf", "/etc/app\x00.conf", "/etc/./app.conf", "/etc/app/", "/proc/self/x"} {
		if _, err := NewAppFile(p, []byte("x")); err == nil {
			t.Errorf("path %q accepted", p)
		}
	}
	if _, err := NewAppFile("/etc/bin.dat", []byte{0xff, 0xfe}); err == nil {
		t.Error("binary content accepted")
	}
}

func TestFileLooksSecretShapes(t *testing.T) {
	secret := map[string]string{ //nolint:gosec // fake values that must be detected as secret
		"json": `{"password": "fake"}`, "ini": "password = fake", "toml": `api_key = "fake"`,
		"env": "PASS=fake", "pem": "-----BEGIN RSA PRIVATE KEY" + "-----\nfake", "url": "dsn: amqp://u:fake@host/v",
	}
	for name, body := range secret {
		if !FileLooksSecret("/etc/app/conf", []byte(body)) {
			t.Errorf("%s shape not flagged", name)
		}
	}
	if FileLooksSecret("/etc/nginx/site.conf", []byte("server { listen 80; }")) {
		t.Error("plain config flagged")
	}
	for _, k := range []string{"PGPASSWORD", "SMTP_PASS", "GHTOKEN"} {
		if !LooksSecret(k) {
			t.Errorf("key %s not secret", k)
		}
	}
	if !ValueLooksSecret("amqp://u:fake@h") || ValueLooksSecret("https://example.test") {
		t.Error("userinfo value detection")
	}
}

func TestCheckComposeHostileShapes(t *testing.T) {
	cases := map[string]string{
		"merge key":   "x-base: &base\n  image: busybox\n  privileged: true\nservices:\n  a:\n    <<: *base\n",
		"parent bind": "services:\n  a:\n    image: busybox\n    volumes:\n      - ../../etc:/host\n",
		"long socket": "services:\n  a:\n    image: busybox\n    volumes:\n      - type: bind\n        source: /run/docker.sock\n        target: /s\n",
	}
	for name, body := range cases {
		if _, blocked := CheckCompose(body); len(blocked) == 0 {
			t.Errorf("%s: not blocked", name)
		}
	}
	// The full parser rejects list-form labels, the lenient host check must still see privileged.
	diff := "services:\n  a:\n    image: busybox\n    labels:\n      - a=b\n    privileged: true\n"
	if got := HostAccessBlocked(diff); len(got) != 1 || !strings.Contains(got[0], "privileged") {
		t.Fatalf("parser differential: %v", got)
	}
}
