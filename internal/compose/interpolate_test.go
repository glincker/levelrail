package compose

import (
	"strings"
	"testing"
)

func TestInterpolateValueForms(t *testing.T) {
	vars := map[string]string{"SET": "v", "EMPTY": ""}
	cases := []struct{ in, want string }{
		{"${SET}", "v"},
		{"${UNSET}", "${UNSET}"},
		{"a-${SET}-b", "a-v-b"},
		{"${UNSET:-d}", "d"},
		{"${EMPTY:-d}", "d"},
		{"${EMPTY-d}", ""},
		{"${UNSET-d}", "d"},
		{"${SET:-d}", "v"},
		{"${SET:+alt}", "alt"},
		{"${EMPTY:+alt}", ""},
		{"${EMPTY+alt}", "alt"},
		{"${UNSET:+alt}", ""},
		{"${SET:?m}", "v"},
		{"${UNSET:-${SET}}", "v"},
		{"${UNSET:-x}:${UNSET2:-y}", "x:y"},
		{"${SERVICE_FQDN_APP:-localhost}", "${SERVICE_FQDN_APP:-localhost}"},
		{"$SERVICE_PASSWORD_DB", "$SERVICE_PASSWORD_DB"},
		{"no vars", "no vars"},
		{"${broken", "${broken"},
		{"$$HOME", dollarSentinel + "HOME"},
	}
	for _, c := range cases {
		got, err := interpolateValue(c.in, vars, func(string) {})
		if err != nil || got != c.want {
			t.Errorf("%q: got %q, %v want %q", c.in, got, err, c.want)
		}
	}
}

func TestInterpolateRequiredErrorsWithoutValues(t *testing.T) {
	vars := map[string]string{"EMPTY": "", "SECRET": "hunter2"}
	for _, in := range []string{"${UNSET:?need it}", "${EMPTY:?need it}", "${UNSET?need it}"} {
		_, err := interpolateValue(in, vars, func(string) {})
		if err == nil || !strings.Contains(err.Error(), "need it") {
			t.Errorf("%q: want error with message, got %v", in, err)
		}
	}
	if _, err := interpolateValue("${EMPTY?m}", vars, func(string) {}); err != nil {
		t.Errorf("set-but-empty with ? must pass: %v", err)
	}
	var warns []string
	_, _ = interpolateValue("${UNSET} ${SECRET}", vars, func(w string) { warns = append(warns, w) })
	if len(warns) != 1 || strings.Contains(warns[0], "hunter2") {
		t.Errorf("warning must name the variable only: %v", warns)
	}
}

func TestResolveMagicVarsInterpolatesEnvOnly(t *testing.T) {
	f, err := Parse([]byte("services:\n  web:\n    image: x:1\n    environment:\n      A: ${PORT:-8080}\n      B: ${NAME:?set NAME}\n      C: cost $$5 ${SERVICE_FQDN_WEB:-h}\n    command: [\"sh\", \"-c\", \"echo ${PORT:-1}\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	f.InterpVars = map[string]string{"NAME": "n"}
	gen := func(string, string, int) (string, error) { return "g", nil }
	per := func(string, string, string) error { return nil }
	if _, _, err := ResolveMagicVars(f, gen, per); err != nil {
		t.Fatal(err)
	}
	env := f.Services["web"].Environment
	if env["A"] != "8080" || env["B"] != "n" || env["C"] != "cost $5 h" {
		t.Fatalf("env = %v", env)
	}
	if got := f.Services["web"].Command[2]; got != "echo ${PORT:-1}" {
		t.Fatalf("command must stay for the container shell, got %q", got)
	}
	f.InterpVars = nil
	if _, _, err := ResolveMagicVars(mustParseReq(t), gen, per); err == nil || !strings.Contains(err.Error(), "set NAME") {
		t.Fatalf("want required-variable error, got %v", err)
	}
}

func mustParseReq(t *testing.T) *File {
	t.Helper()
	f, err := Parse([]byte("services:\n  web:\n    image: x:1\n    environment:\n      B: ${NAME:?set NAME}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
