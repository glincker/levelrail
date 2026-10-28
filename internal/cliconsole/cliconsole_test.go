package cliconsole

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

func TestParseLine(t *testing.T) {
	tests := []struct {
		line    string
		want    []string
		wantErr bool
	}{
		{"apps list", []string{"apps", "list"}, false},
		{"  apps   get  web  ", []string{"apps", "get", "web"}, false},
		{`apps env set web "A=hello world"`, []string{"apps", "env", "set", "web", "A=hello world"}, false},
		{`iam policies create --document '{"Statement":[]}'`, []string{"iam", "policies", "create", "--document", `{"Statement":[]}`}, false},
		{`logs search web --regex 'a|b'`, []string{"logs", "search", "web", "--regex", "a|b"}, false},
		{`apps get we\ b`, []string{"apps", "get", "we b"}, false},
		{`apps get "a\"b"`, []string{"apps", "get", `a"b`}, false},
		{"apps list | sh", nil, true},
		{"apps list; rm -rf /", nil, true},
		{"apps list && id", nil, true},
		{"apps list & id", nil, true},
		{"apps list > /etc/passwd", nil, true},
		{"apps list < /etc/passwd", nil, true},
		{"apps get `id`", nil, true},
		{"apps get $(id)", nil, true},
		{`apps get "$(id)"`, nil, true},
		{"apps get ${HOME}", nil, true},
		{"apps list\nid", nil, true},
		{"apps list\rid", nil, true},
		{"apps get \x00x", nil, true},
		{"apps get \x1b[31m", nil, true},
		{"FOO=bar apps list", nil, true},
		{"APP_API_URL=http://evil apps list", nil, true},
		{`apps get "unterminated`, nil, true},
		{`apps get trailing\`, nil, true},
		{"(apps list)", nil, true},
		{"", nil, true},
		{"   ", nil, true},
		{strings.Repeat("a ", MaxArgs+1), nil, true},
		{strings.Repeat("a", MaxLineBytes+1), nil, true},
	}
	for _, tc := range tests {
		got, err := ParseLine(tc.line)
		if (err != nil) != tc.wantErr {
			t.Fatalf("ParseLine(%q) err = %v, wantErr %v", tc.line, err, tc.wantErr)
		}
		if err != nil && !IsReject(err) {
			t.Fatalf("ParseLine(%q) err %v is not a RejectError", tc.line, err)
		}
		if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseLine(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		line     string
		wantPath string
		wantErr  bool
	}{
		{"apps list", "apps list", false},
		{"apps list --json", "apps list", false},
		{"db list", "databases list", false},
		{"apps deploys list web", "apps deploys list", false},
		{"auth whoami", "auth whoami", false},
		{"upgrade --plan", "upgrade", true},
		{"help", "help", false},
		{"apps get web --team ops", "apps get", false},
		{"nosuch", "", true},
		{"sh -c id", "", true},
		{"/bin/sh", "", true},
		{"auth login", "auth login", true},
		{"auth", "auth", true},
		{"completion bash", "", true},
		{"profile list", "profile list", true},
		{"tui", "tui", true},
		{"init --dir /", "", true},
		{"proxy setup", "proxy setup", true},
		{"import ./compose.yaml", "import", true},
		{"apply -f -", "", true},
		{"export", "export", true},
		{"update", "update", true},
		{"tokens create --name x", "tokens create", true},
		{"pipelines validate /etc/passwd", "pipelines validate", true},
		{"apps exec web -- sh", "", true},
		{"apps exec web id", "apps exec", true},
		{"backups download db", "backups download", true},
		{"assets upload ./logo.png", "assets upload", true},
		{"secrets generate-master-key", "secrets generate-master-key", true},
		{"control-plane-backups keys generate", "control-plane-backups keys generate", true},
		{"apps --team ops exec web id", "", true},
		{"apps --team=ops exec web id", "", true},
		{"--project p apps exec web", "", true},
		{"apps --json exec web", "", true},
		{"apps env import web --file /var/lib/data/master.key", "", true},
		{"apps env import web -file=/etc/shadow", "", true},
		{"apps env export web --out /tmp/x", "", true},
		{"apps list --api-url http://169.254.169.254", "", true},
		{"apps list --token abc", "", true},
		{"apps list --debug", "", true},
		{"apps create --interactive", "", true},
		{"apps secrets set web --env-file .env", "", true},
		{"domains tls-cert set x --cert-file a --key-file b", "", true},
		{"iam policies create --name p --document file:///etc/passwd", "", true},
		{"iam policies create --name p --document=FILE://x", "", true},
		{"models deploy m --hf-token-from-env HOME", "", true},
		{"audit-log --output-file /tmp/a", "", true},
		// Review findings: host reads, writes and launches stay refused.
		{"deploy /data --static --include master.key --app x --yes", "", true},
		{"deploy /data --app x --yes", "deploy", true},
		{"bench results upload /data/x.json", "bench results upload", true},
		{"storage buckets upload b k /data/master.key", "", true},
		{"apps log-pipeline preview web --input /data/x", "", true},
		{"apps log-pipeline preview web", "apps log-pipeline preview", true},
		{"plugins install x --manifest /data/m", "", true},
		{"static-routes set web --redirects /data/r", "", true},
		{"sso saml set --idp-cert-file /data/c", "", true},
		{"images lint /data/Dockerfile --fix", "", true},
		{"bench soak --csv /tmp/x", "", true},
		{"dev --bin /bin/sh", "", true},
		{"dev", "dev", true},
		{"functions dev /data --image alpine", "", true},
		{"databases connect pg --launch", "", true},
		{"databases connect pg", "databases connect", true},
		{"apps get web --x=@/data/master.key", "", true},
		{"apps get @/data/master.key", "", true},
		{"apps get -", "", true},
		{"apps get web --name=-", "", true},
		{"apps get web --some-path /x", "", true},
		{"apps list --cache-dir /x", "", true},
		{"apps logs web --follow", "apps logs", false},
		{"apps restart web", "apps restart", false},
	}
	for _, tc := range tests {
		argv, err := ParseLine(tc.line)
		if err != nil {
			t.Fatalf("ParseLine(%q): %v", tc.line, err)
		}
		path, err := Check(argv)
		if (err != nil) != tc.wantErr {
			t.Fatalf("Check(%q) err = %v, wantErr %v", tc.line, err, tc.wantErr)
		}
		if tc.wantPath != "" && path != tc.wantPath {
			t.Fatalf("Check(%q) path = %q, want %q", tc.line, path, tc.wantPath)
		}
	}
}

// TestCheckDeniesByDefault walks the whole CLI tree: only allowlisted paths pass.
func TestCheckDeniesByDefault(t *testing.T) {
	var walk func(prefix []string, nodes map[string]*clitree.Node)
	walk = func(prefix []string, nodes map[string]*clitree.Node) {
		for name, n := range nodes {
			words := append(append([]string(nil), prefix...), name)
			path := canonical(words[0])
			if len(words) > 1 {
				path += " " + strings.Join(words[1:], " ")
			}
			_, err := Check(words)
			if want := allowed[path]; want != (err == nil) {
				t.Errorf("Check(%q) err = %v, allowlisted = %v", path, err, want)
			}
			if n != nil {
				walk(words, n.Subs)
			}
		}
	}
	walk(nil, clitree.Tree)
	seen := map[string]bool{}
	for _, e := range Allowlist() {
		if e.Reason == "" || seen[e.Path] {
			t.Errorf("allowlist entry %q is duplicated or has no reason", e.Path)
		}
		seen[e.Path] = true
		if !treeHas(e.Path) {
			t.Errorf("allowlist entry %q is not a CLI command", e.Path)
		}
	}
}

func treeHas(path string) bool {
	words := strings.Fields(path)
	nodes := clitree.Tree
	for i, w := range words {
		n, ok := nodes[w]
		if !ok {
			return false
		}
		if i == len(words)-1 {
			return true
		}
		if n == nil {
			return false
		}
		nodes = n.Subs
	}
	return false
}

func TestCommandsHideDenied(t *testing.T) {
	byName := map[string]Command{}
	for _, c := range Commands() {
		byName[c.Name] = c
	}
	for _, denied := range []string{"completion", "tui", "profile", "import", "apply", "init"} {
		if _, ok := byName[denied]; ok {
			t.Fatalf("Commands() offers denied %q", denied)
		}
	}
	if auth, ok := byName["auth"]; !ok || !reflect.DeepEqual(auth.Subcommands, []string{"whoami"}) {
		t.Fatalf("auth = %+v, want only whoami", auth)
	}
	for _, s := range byName["apps"].Subcommands {
		if s == "exec" {
			t.Fatal("apps exec offered")
		}
	}
	if !sort.SliceIsSorted(Commands(), func(i, j int) bool { return Commands()[i].Name < Commands()[j].Name }) {
		t.Fatal("Commands() not sorted")
	}
}

func TestHostNeverReachesCLI(t *testing.T) {
	for _, argv := range [][]string{{"host", "df"}, {"host"}, {"--team", "t1", "host", "df"}, {"--project=p", "host", "free"}} {
		if _, err := Check(argv); err == nil || !IsReject(err) {
			t.Errorf("Check(%q) = %v, want a refusal", argv, err)
		}
	}
	if !IsHost([]string{"host", "df"}) || IsHost([]string{"apps", "host"}) || IsHost(nil) {
		t.Fatal("IsHost routes the wrong lines")
	}
}

func TestCommandsHostToggle(t *testing.T) {
	find := func() (Command, bool) {
		for _, c := range Commands() {
			if c.Name == "host" {
				return c, true
			}
		}
		return Command{}, false
	}
	t.Setenv("APP_CLI_CONSOLE_HOST", "")
	host, ok := find()
	want := []string{"df", "free", "help", "load", "os", "service", "uptime"}
	if !ok || !reflect.DeepEqual(host.Subcommands, want) {
		t.Fatalf("host = %+v, want %v", host, want)
	}
	t.Setenv("APP_CLI_CONSOLE_HOST", "off")
	if _, ok := find(); ok {
		t.Fatal("host offered with APP_CLI_CONSOLE_HOST=off")
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		line, path, want string
	}{
		{"apps list --json", "apps list", "apps list --json"},
		{"databases connect --name x --password hunter2", "databases connect", "databases connect --name x --password [redacted]"},
		{"databases connect --name x --password=hunter2", "databases connect", "databases connect --name x --password=[redacted]"},
		{"apps env set web DB_PASSWORD=hunter2", "apps env", "apps env set [redacted] [redacted]"},
		{"apps secrets set web API_KEY s3cret", "apps secrets set", "apps secrets set web [redacted] [redacted]"},
		{"channels create --url https://user:pw@hooks.example/x", "channels create", "channels create --url https://%5Bredacted%5D@hooks.example/x"},
		{"flags set k API_TOKEN=abc", "flags set", "flags set k API_TOKEN=[redacted]"},
	}
	for _, tc := range tests {
		argv, err := ParseLine(tc.line)
		if err != nil {
			t.Fatal(err)
		}
		got := Redact(argv, tc.path)
		if got != tc.want {
			t.Fatalf("Redact(%q) = %q, want %q", tc.line, got, tc.want)
		}
		if strings.Contains(got, "hunter2") || strings.Contains(got, "s3cret") {
			t.Fatalf("Redact(%q) leaked a secret: %q", tc.line, got)
		}
	}
}

func TestEnvIsScrubbed(t *testing.T) {
	t.Setenv("APP_MASTER_KEY", "should-not-leak")
	env := Env("http://127.0.0.1:8080", "tok", "/tmp/h", "")
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "APP_API_URL", "APP_API_TOKEN", "HOME", "TERM", "NO_COLOR", "LANG":
		default:
			t.Fatalf("unexpected child env var %q", kv)
		}
	}
}

func writeFakeCLI(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake-cli")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	return p
}

func TestRunnerScrubsEnvAndCapsOutput(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("credential drop needs a real unprivileged user")
	}
	t.Setenv("APP_MASTER_KEY", "should-not-leak")
	bin := writeFakeCLI(t, `/usr/bin/env; pwd; echo "args:$*"; exit 3`)
	r := &Runner{Binary: bin, APIURL: "http://127.0.0.1:1", Timeout: 5 * time.Second, MaxOutputBytes: 1 << 20}
	var out strings.Builder
	res, err := r.Run(context.Background(), []string{"apps", "list"}, "lr_session_tok", func(_ string, d []byte) error {
		out.Write(d)
		return nil
	})
	if err != nil || res.ExitCode != 3 {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	got := out.String()
	if strings.Contains(got, "should-not-leak") || strings.Contains(got, "lr_session_tok") || !strings.Contains(got, "APP_API_TOKEN=[redacted]") || !strings.Contains(got, "args:apps list") {
		t.Fatalf("child output = %q", got)
	}

	big := writeFakeCLI(t, `while :; do echo xxxxxxxxxxxxxxxx; done`)
	r = &Runner{Binary: big, APIURL: "x", Timeout: 5 * time.Second, MaxOutputBytes: 1000}
	var n int
	res, err = r.Run(context.Background(), nil, "tok", func(_ string, d []byte) error { n += len(d); return nil })
	if err != nil || !res.Truncated || n > 1200 {
		t.Fatalf("cap: res = %+v err = %v bytes = %d", res, err, n)
	}

	slow := writeFakeCLI(t, `sleep 30`)
	r = &Runner{Binary: slow, APIURL: "x", Timeout: 200 * time.Millisecond, MaxOutputBytes: 1000}
	start := time.Now()
	res, err = r.Run(context.Background(), nil, "tok", func(string, []byte) error { return nil })
	if err != nil || !res.TimedOut || time.Since(start) > 5*time.Second {
		t.Fatalf("timeout: res = %+v err = %v after %s", res, err, time.Since(start))
	}
}

func TestVetBinary(t *testing.T) {
	good := writeFakeCLI(t, "exit 0")
	if _, err := vetBinary(good); err != nil {
		t.Fatalf("good binary refused: %v", err)
	}
	if _, err := vetBinary("relative/cli"); err == nil {
		t.Fatal("relative path accepted")
	}
	writable := writeFakeCLI(t, "exit 0")
	if err := os.Chmod(writable, 0o777); err != nil { //nolint:gosec // asserting the refusal
		t.Fatal(err)
	}
	if _, err := vetBinary(writable); err == nil {
		t.Fatal("world writable binary accepted")
	}
	if _, err := vetBinary(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing binary accepted")
	}
}

func TestSanitizeLine(t *testing.T) {
	tok := "lr_sessiontoken123"
	tests := []struct{ in, want string }{
		{"\x1b[31mred\x1b[0m done", "red done"},
		{"\x1b]0;title\x07after", "after"},
		{"\x1b]8;;http://evil\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"a\u009b2Jb", "ab"},
		{"bell\x07 and \x08back", "bell and back"},
		{"token is " + tok, "token is [redacted]"},
		{"DB_PASSWORD=hunter2 other=ok", "DB_PASSWORD=[redacted] other=ok"},
		{`{"api_key": "abc123", "name": "web"}`, `{"api_key": "[redacted]", "name": "web"}`},
		{"postgres://app:s3cret@db:5432/x", "postgres://app:[redacted]@db:5432/x"},
		{"Authorization: Bearer abc.def", "Authorization: Bearer [redacted]"},
		{"-----BEGIN RSA PRIVATE KEY-----", "[redacted]"},
		{"plain output", "plain output"},
	}
	for _, tc := range tests {
		if got := SanitizeLine(tc.in, 0, tok); got != tc.want {
			t.Errorf("SanitizeLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := SanitizeLine(strings.Repeat("x", 50), 10); got != strings.Repeat("x", 10)+"..." {
		t.Errorf("line cap: got %q", got)
	}
}

func TestLineFilterJoinsSplitReads(t *testing.T) {
	f := &lineFilter{maxLine: 64, secrets: []string{"lr_tok"}}
	var out []byte
	for _, chunk := range []string{"ok \x1b[3", "1mred\x1b[0m lr_", "tok\nsecond", " line"} {
		out = append(out, f.write([]byte(chunk))...)
	}
	out = append(out, f.flush()...)
	if got, want := string(out), "ok red [redacted]\nsecond line"; got != want {
		t.Fatalf("filtered = %q, want %q", got, want)
	}
	long := &lineFilter{maxLine: 8}
	got := string(long.write([]byte(strings.Repeat("y", 20)))) + string(long.write([]byte("zz\nnext\n")))
	if got != "yyyyyyyy...\nnext\n" {
		t.Fatalf("long line = %q", got)
	}
}
