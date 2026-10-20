package secretscan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixtures are assembled at runtime so no token-shaped literal is committed.
func fixtures() map[string]string {
	return map[string]string{
		"aws-access-key-id":         "AK" + "IA" + strings.Repeat("Q7", 8),
		"github-token":              "gh" + "p_" + strings.Repeat("aB3", 12),
		"gitlab-token":              "gl" + "pat-" + strings.Repeat("x9Yz", 6),
		"slack-token":               "xo" + "xb-" + "1234567890-abcdefghij",
		"stripe-secret-key":         "sk" + "_live_" + strings.Repeat("a1B2", 6),
		"npm-token":                 "np" + "m_" + strings.Repeat("k3Lm", 9),
		"gcp-api-key":               "AI" + "za" + strings.Repeat("A1b_", 8) + "xyz",
		"jwt":                       "ey" + "Jhbgcioijiuzi1ni.ey" + "Jzdwiioij9abcdef.abcdEFGH1234",
		"private-key":               "-----BEGIN " + "RSA PRIVATE" + " KEY-----",
		"slack-webhook":             "https://hooks." + "slack.com/services/T0ABC/B0DEF/" + strings.Repeat("z", 12),
		"url-credentials":           "postgres://app:" + "Sup3rS3cretPw" + "@db.internal:5432/app",
		"aws-secret-access-key":     "aws_secret_access_key = " + strings.Repeat("aB1/", 10),
		"github-fine-grained-token": "github" + "_pat_" + strings.Repeat("A1b2C3", 5),
	}
}

func TestMatchRulesTruePositives(t *testing.T) {
	for rule, text := range fixtures() {
		t.Run(rule, func(t *testing.T) {
			var got bool
			for _, m := range matchRules("value = " + text) {
				if m.rule == rule {
					got = true
				}
			}
			if !got {
				t.Fatalf("rule %s did not match its fixture", rule)
			}
		})
	}
}

func TestMatchRulesFalsePositives(t *testing.T) {
	cases := map[string]string{ //nolint:gosec // negative fixtures, not credentials
		"plain prose":       "this is a normal sentence about keys and tokens",
		"short akia":        "AKIA1234",
		"placeholder url":   "postgres://user:${DB_PASSWORD}@db:5432/app",
		"url without creds": "https://example.com/path?x=1",
		"jwt lookalike":     "eyJ.eyJ.x",
		"git sha":           "commit 9fceb02d0ae598e95dc970b74767f19372d61af8",
		"word placeholder":  "postgres://app:changeme-please@db/app",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if ms := matchRules(text); len(ms) != 0 {
				t.Fatalf("unexpected match %v", ms)
			}
		})
	}
}

func TestEntropyRule(t *testing.T) {
	high := "q8Zx" + "L2mP9vTn" + "B4kR7wYc5H"
	cases := []struct {
		name, line string
		want       bool
	}{
		{"named high entropy", "SESSION_SECRET=" + high, true},
		{"yaml style", `api_token: "` + high + `"`, true},
		{"name not sensitive", "GREETING=" + high, false},
		{"public prefix", "NEXT_PUBLIC_API_KEY=" + high, false},
		{"low entropy", "SESSION_SECRET=" + strings.Repeat("a", 24), false},
		{"reference", "SESSION_SECRET=${" + strings.Repeat("A_B", 8) + "}", false},
		{"too short", "SESSION_SECRET=abc123", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := len(entropyMatches(c.line)) > 0; got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("jwt", "value-one")
	if a != Fingerprint("jwt", "value-one") {
		t.Fatal("fingerprint is not deterministic")
	}
	if a == Fingerprint("jwt", "value-two") || a == Fingerprint("npm-token", "value-one") {
		t.Fatal("fingerprint collides across value or rule")
	}
	if len(a) != fingerprintLength || strings.Contains(a, "value") {
		t.Fatalf("bad fingerprint %q", a)
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestScanDirFindsAndNeverLeaksValue(t *testing.T) {
	tok := fixtures()["github-token"]
	root := writeTree(t, map[string]string{
		"src/app.env":          "A=1\nTOKEN=" + tok + "\n",
		"node_modules/x/k.env": "TOKEN=" + tok,
		"docs/readme.md":       "nothing here",
		"bin/blob":             "\x00\x01" + tok,
		"deep/dir/config.yaml": "k: v\n",
		"package-lock.json":    tok,
		"images/logo.png":      tok,
	})
	res, err := ScanDir(context.Background(), root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("want 1 finding, got %+v", res.Findings)
	}
	f := res.Findings[0]
	if f.RuleID != "github-token" || f.Path != "src/app.env" || f.Line != 2 || f.Source != SourceFile {
		t.Fatalf("unexpected finding %+v", f)
	}
	if strings.Contains(f.Fingerprint, tok) || f.Fingerprint != Fingerprint("github-token", tok) {
		t.Fatalf("bad fingerprint %q", f.Fingerprint)
	}
}

func TestScanDirCaps(t *testing.T) {
	tok := fixtures()["npm-token"]
	files := map[string]string{"big.txt": strings.Repeat("x", 4096) + tok}
	for i := 0; i < 10; i++ {
		files["f"+string(rune('a'+i))+".txt"] = "TOKEN=" + tok
	}
	root := writeTree(t, files)

	t.Run("file size cap skips large file", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxFileBytes = 1024
		res, err := ScanDir(context.Background(), root, lim)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range res.Findings {
			if f.Path == "big.txt" {
				t.Fatal("scanned a file over the size cap")
			}
		}
		if res.FilesSkipped == 0 {
			t.Fatal("expected a skipped file")
		}
	})
	t.Run("file count cap truncates", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxFiles = 3
		res, err := ScanDir(context.Background(), root, lim)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Truncated || res.FilesScanned != 3 {
			t.Fatalf("truncated=%v scanned=%d", res.Truncated, res.FilesScanned)
		}
	})
	t.Run("finding cap truncates", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxFindings = 2
		res, err := ScanDir(context.Background(), root, lim)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Truncated || len(res.Findings) != 2 {
			t.Fatalf("truncated=%v findings=%d", res.Truncated, len(res.Findings))
		}
	})
	t.Run("total bytes cap truncates", func(t *testing.T) {
		lim := DefaultLimits()
		lim.MaxTotalBytes = 100
		res, err := ScanDir(context.Background(), root, lim)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Truncated {
			t.Fatal("expected truncation")
		}
	})
}

func TestScanDirDoesNotFollowSymlinks(t *testing.T) {
	outside := writeTree(t, map[string]string{"secret.env": "TOKEN=" + fixtures()["npm-token"]})
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := os.Symlink(filepath.Join(outside, "secret.env"), filepath.Join(root, "file-link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	res, err := ScanDir(context.Background(), root, DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("followed a symlink out of the build context: %+v", res.Findings)
	}
}

func TestScanDirErrors(t *testing.T) {
	if _, err := ScanDir(context.Background(), filepath.Join(t.TempDir(), "missing"), DefaultLimits()); err == nil {
		t.Fatal("missing root must error")
	}
	root := writeTree(t, map[string]string{"a.txt": "x"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if _, err := ScanDir(ctx, root, DefaultLimits()); err == nil {
		t.Fatal("expired context must error")
	}
}

func TestScanEnv(t *testing.T) {
	fx := fixtures()
	res := ScanEnv(map[string]string{
		"STRIPE":      fx["stripe-secret-key"],
		"GREETING":    "hello",
		"SESSION_KEY": "q8Zx" + "L2mP9vTn" + "B4kR7wYc5H",
		"EMPTY":       "",
	}, DefaultLimits())
	got := map[string]string{}
	for _, f := range res.Findings {
		if f.Source != SourceEnv {
			t.Fatalf("env finding has source %q", f.Source)
		}
		got[f.Var] = f.RuleID
	}
	if got["STRIPE"] != "stripe-secret-key" || got["SESSION_KEY"] != RuleHighEntropy || len(got) != 2 {
		t.Fatalf("unexpected env findings %v", got)
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{envMaxFiles: "7", envTimeout: "2s", envMaxFindings: "bogus", envMaxFileBytes: "-1"}
	l := LimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	d := DefaultLimits()
	if l.MaxFiles != 7 || l.Timeout != 2*time.Second || l.MaxFindings != d.MaxFindings || l.MaxFileBytes != d.MaxFileBytes {
		t.Fatalf("unexpected limits %+v", l)
	}
}
