package compose

import (
	"errors"
	"strings"
	"testing"
)

func TestStrictInterpolateDeclaredOnly(t *testing.T) {
	t.Setenv("LR_COMPOSE_CANARY", "host-canary-value")
	cases := []struct {
		in, want, code string
		env            map[string]string
	}{
		{"${LR_COMPOSE_CANARY}", "", CodeStrictVarMissing, nil},
		{"${LR_COMPOSE_CANARY:-fallback}", "fallback", "", nil},
		{"pg://${HOST}:5432", "pg://db:5432", "", map[string]string{"HOST": "db"}},
		{"$${HOME}", "${HOME}", "", nil},
		{"${REQ:?set it}", "", CodeStrictVarMissing, nil},
	}
	for _, tc := range cases {
		got, err := InterpolateDeclared(tc.in, tc.env)
		if strings.Contains(got, "host-canary-value") || (err != nil && strings.Contains(err.Error(), "host-canary-value")) {
			t.Fatalf("%q: host env leaked", tc.in)
		}
		if tc.code != "" {
			var se *StrictError
			if !errors.As(err, &se) || se.Code != tc.code {
				t.Fatalf("%q: want %s, got %v", tc.in, tc.code, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got %q, %v", tc.in, got, err)
		}
	}
}

func TestStrictCheckDeclaredVars(t *testing.T) {
	t.Setenv("LR_COMPOSE_CANARY", "host-canary-value")
	src := []byte("services:\n  web:\n    image: nginx:${TAG}\n    environment:\n      A: ${LR_COMPOSE_CANARY}\n")
	err := CheckDeclaredVars(src, "compose/web.yaml", StrictLimits{}, map[string]string{"TAG": "x"})
	var se StrictErrors
	if !errors.As(err, &se) || len(se) != 1 || se[0].Line != 5 || se[0].Key != "LR_COMPOSE_CANARY" {
		t.Fatalf("got %v", err)
	}
}

func TestStrictSecretScanUsesMatcher(t *testing.T) {
	src := []byte("services:\n  web:\n    image: nginx\n    environment:\n      - TOKEN=${TOKEN}\n      - MODE=prod\n    labels:\n      flagged: \"x\"\n")
	var seen []string
	err := ScanSecretLiterals(src, "c.yaml", StrictLimits{}, func(k, _ string) bool {
		seen = append(seen, k)
		return k == "flagged"
	})
	var se StrictErrors
	if !errors.As(err, &se) || len(se) != 1 || se[0].Key != "labels.flagged" || se[0].Line != 8 {
		t.Fatalf("got %v", err)
	}
	for _, k := range seen {
		if k == "TOKEN" {
			t.Fatal("a pure ${VAR} value must not be judged by its key name")
		}
	}
}
