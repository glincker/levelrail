package compose

import (
	"strings"
	"testing"
	"time"
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

func TestInterpolateBoundsNestingAndLength(t *testing.T) {
	deep := strings.Repeat("${A:-", 100_000) + "x" + strings.Repeat("}", 100_000)
	start := time.Now()
	_, err := interpolateValue(deep, nil, func(string) {})
	if err == nil {
		t.Fatal("100k nesting must be rejected")
	}
	took := time.Since(start)
	t.Logf("100k nesting rejected in %s: %v", took, err)
	if took > 100*time.Millisecond {
		t.Fatalf("took %s, want under 100ms", took)
	}

	// 10k levels fit under the length cap, so this one reaches the depth check.
	under := strings.Repeat("${A:-", 10_000) + "x" + strings.Repeat("}", 10_000)
	start = time.Now()
	if _, err := interpolateValue(under, nil, func(string) {}); err == nil || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("10k nesting: want a depth error, got %v", err)
	}
	t.Logf("10k nesting rejected in %s", time.Since(start))
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("deep nesting too slow")
	}

	ok := strings.Repeat("${A:-", maxInterpolationDepth) + "x" + strings.Repeat("}", maxInterpolationDepth)
	if got, err := interpolateValue(ok, nil, func(string) {}); err != nil || got != "x" {
		t.Fatalf("depth at the cap: %q, %v", got, err)
	}
	long := strings.Repeat("a", maxInterpolatedLen)
	if got, err := interpolateValue(long, nil, func(string) {}); err != nil || len(got) != maxInterpolatedLen {
		t.Fatalf("value at the cap: %v", err)
	}
	if _, err := interpolateValue(long+"a", nil, func(string) {}); err == nil {
		t.Fatal("value over the cap must fail")
	}
	big := map[string]string{"B": strings.Repeat("b", 40<<10)}
	if _, err := interpolateValue("${B}${B}", big, func(string) {}); err == nil {
		t.Fatal("expansion over the cap must fail")
	}
}
