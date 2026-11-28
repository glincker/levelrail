package compose

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestResolveMagicVars_DollarEscapeInArgv(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"var", `exec x "$$VAR"`, `exec x "$VAR"`},
		{"double", "a$$$$b", "a$$b"},
		{"brace", "$${VAR}", "${VAR}"},
		{"escaped magic stays literal", "$$SERVICE_PASSWORD_X", "$SERVICE_PASSWORD_X"},
		{"escaped brace magic stays literal", "$${SERVICE_PASSWORD_X}", "${SERVICE_PASSWORD_X}"},
		{"no dollar", "plain text", "plain text"},
		{"single dollar unknown", "echo $HOME", "echo $HOME"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := "services:\n  a:\n    image: x:1\n    entrypoint: [\"sh\", \"-c\", '" + tc.in + "']\n" +
				"    command: [\"sh\", \"-c\", '" + tc.in + "']\n" +
				"    healthcheck:\n      test: [\"CMD-SHELL\", '" + tc.in + "']\n"
			f, err := Parse([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			gen := func(string, string, int) (string, error) { return "SECRET", nil }
			persist := func(string, string, string) error { return nil }
			_, unresolved, err := ResolveMagicVars(f, gen, persist)
			if err != nil || len(unresolved) != 0 {
				t.Fatalf("err=%v unresolved=%v", err, unresolved)
			}
			svc := f.Services["a"]
			got := []string{svc.Entrypoint[2], svc.Command[2], svc.Healthcheck.Test[1]}
			for i, g := range got {
				if g != tc.want {
					t.Errorf("field %d = %q, want %q", i, g, tc.want)
				}
			}
		})
	}
}

func TestResolveMagicVars_UnescapedMagicInCommandStillUnresolved(t *testing.T) {
	body := "services:\n  a:\n    image: x:1\n    command: [\"sh\", \"-c\", \"echo $SERVICE_PASSWORD_X\"]\n"
	f, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	gen := func(string, string, int) (string, error) { return "SECRET", nil }
	_, unresolved, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil })
	if err != nil || len(unresolved) != 1 {
		t.Fatalf("err=%v unresolved=%v, want secrets in argv kept unresolved", err, unresolved)
	}
}

func resolveBody(t *testing.T, body string) *File {
	t.Helper()
	f, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	gen := func(string, string, int) (string, error) { return "SECRET", nil }
	if _, un, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil }); err != nil || len(un) != 0 {
		t.Fatalf("err=%v unresolved=%v", err, un)
	}
	return f
}

func TestFromDesiredServices_DollarEscapeRoundTrip(t *testing.T) {
	body := "services:\n  a:\n    image: x:1\n" +
		"    entrypoint: [\"sh\", \"-c\", 'echo $$VAR $$$$ $${X}']\n" +
		"    command: [\"sh\", \"-c\", 'echo $$VAR $$$$ $${X}']\n"
	f1 := resolveBody(t, body)
	ds, _, err := ToDesiredServices("app", f1)
	if err != nil {
		t.Fatal(err)
	}
	out, err := FromDesiredServices("app", ds)
	if err != nil {
		t.Fatal(err)
	}
	f2 := resolveBody(t, out)
	a, b := f1.Services["a"], f2.Services["a"]
	if a.Command[2] != "echo $VAR $$ ${X}" || a.Command[2] != b.Command[2] || a.Entrypoint[2] != b.Entrypoint[2] {
		t.Fatalf("round trip changed argv: %q vs %q", a.Command, b.Command)
	}
}

func TestResolveMagicVars_DollarEscapeEdges(t *testing.T) {
	t.Run("escape then magic in command is rejected", func(t *testing.T) {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    command: [\"sh\", \"-c\", \"echo $$$SERVICE_PASSWORD_X\"]\n"))
		if err != nil {
			t.Fatal(err)
		}
		gen := func(string, string, int) (string, error) { return "SECRET", nil }
		_, un, _ := ResolveMagicVars(f, gen, func(string, string, string) error { return nil })
		if len(un) != 1 {
			t.Fatalf("unresolved = %v, want the secret rejected", un)
		}
	})
	t.Run("fqdn escape", func(t *testing.T) {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    command: [\"$${SERVICE_FQDN_X}\", \"$$${SERVICE_FQDN_X}\"]\n"))
		if err != nil {
			t.Fatal(err)
		}
		f.Domains = map[string]string{"a": "h.example.com"}
		gen := func(string, string, int) (string, error) { return "SECRET", nil }
		if _, un, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil }); err != nil || len(un) != 0 {
			t.Fatalf("err=%v un=%v", err, un)
		}
		c := f.Services["a"].Command
		if c[0] != "${SERVICE_FQDN_X}" || c[1] != "$h.example.com" {
			t.Fatalf("command = %q", c)
		}
	})
	t.Run("magic in entrypoint and healthcheck stays literal", func(t *testing.T) {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    entrypoint: [\"sh\", \"-c\", \"echo $SERVICE_PASSWORD_X\"]\n    healthcheck:\n      test: [\"CMD-SHELL\", \"echo $SERVICE_PASSWORD_X\"]\n"))
		if err != nil {
			t.Fatal(err)
		}
		gen := func(string, string, int) (string, error) { t.Fatal("generate called"); return "", nil }
		if _, _, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil }); err != nil {
			t.Fatal(err)
		}
		s := f.Services["a"]
		if s.Entrypoint[2] != "echo $SERVICE_PASSWORD_X" || s.Healthcheck.Test[1] != "echo $SERVICE_PASSWORD_X" {
			t.Fatalf("got %q %q", s.Entrypoint, s.Healthcheck.Test)
		}
	})
	t.Run("user supplied sentinel is stripped everywhere", func(t *testing.T) {
		in := `"\u0000$\u0000"`
		f := resolveBody(t, "services:\n  a:\n    image: x:1\n    user: "+in+"\n    working_dir: "+in+"\n    entrypoint: ["+in+"]\n    command: ["+in+"]\n    healthcheck:\n      test: [\"CMD-SHELL\", "+in+"]\n")
		s := f.Services["a"]
		for _, v := range []string{s.User, s.WorkingDir, s.Entrypoint[0], s.Command[0], s.Healthcheck.Test[1]} {
			if strings.ContainsRune(v, 0) {
				t.Errorf("value %q still carries a NUL", v)
			}
		}
	})
	t.Run("probe exec argv is unescaped", func(t *testing.T) {
		for _, test := range []string{`["CMD", "echo", "$$X"]`, `["CMD-SHELL", "echo $$X"]`} {
			f := resolveBody(t, "services:\n  a:\n    image: x:1\n    healthcheck:\n      test: "+test+"\n")
			ds, _, err := ToDesiredServices("app", f)
			if err != nil {
				t.Fatal(err)
			}
			var exec *store.ServiceProbe
			if ds[0].Health != nil {
				exec = ds[0].Health.Readiness
			}
			if exec == nil || !strings.Contains(strings.Join(exec.Exec, " "), "$X") || strings.Contains(strings.Join(exec.Exec, " "), "$$") {
				t.Errorf("%s: probe = %+v", test, exec)
			}
		}
	})
	t.Run("env double escape and embedded secret", func(t *testing.T) {
		f, err := Parse([]byte("services:\n  a:\n    image: x:1\n    environment:\n      A: \"$$$$\"\n      B: \"$$x-$SERVICE_PASSWORD_X\"\n"))
		if err != nil {
			t.Fatal(err)
		}
		persisted := map[string]string{}
		gen := func(string, string, int) (string, error) { return "SECRET", nil }
		persist := func(_, k, v string) error { persisted[k] = v; return nil }
		if _, _, err := ResolveMagicVars(f, gen, persist); err != nil {
			t.Fatal(err)
		}
		if got := f.Services["a"].Environment["A"]; got != "$$" {
			t.Errorf("A = %q, want $$", got)
		}
		if persisted["B"] != "$x-SECRET" {
			t.Errorf("B = %q, want $x-SECRET", persisted["B"])
		}
	})
}

func TestNotices_DollarEscape(t *testing.T) {
	f := resolveBody(t, "services:\n  a:\n    image: x:1\n    command: [\"sh\", \"-c\", \"echo $$X\"]\n  b:\n    image: x:1\n")
	var n int
	for _, no := range f.Notices() {
		if strings.Contains(no.Message, "$$") {
			n++
			if no.Service != "a" {
				t.Errorf("notice on %q", no.Service)
			}
		}
	}
	if n != 1 {
		t.Fatalf("got %d dollar notices, want 1", n)
	}
}
