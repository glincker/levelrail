package main

import (
	"strings"
	"testing"
)

func TestEnvRoundTrip_QuotedAndMultiline(t *testing.T) {
	pem := "-----BEGIN DUMMY KEY-----\nAAAA1111\nBBBB2222\n-----END DUMMY KEY-----"
	cases := []struct {
		name, src, want string
	}{
		{"double quoted", `K="two words"`, "two words"},
		{"single quoted", `K='two words'`, "two words"},
		{"double with hash and equals", `K="a#b=c d"`, "a#b=c d"},
		{"single with hash and equals", `K='a#b=c d'`, "a#b=c d"},
		{"single keeps backslash n", `K='a\nb'`, `a\nb`},
		{"double escapes", `K="a\"b\\c\nd"`, "a\"b\\c\nd"},
		{"multiline pem", "K=\"" + pem + "\"", pem},
		{"multiline single quoted", "K='" + pem + "'", pem},
		{"trailing spaces preserved", `K="  pad  "`, "  pad  "},
		{"trailing spaces unquoted trimmed", "K=plain   ", "plain"},
		{"empty", "K=", ""},
		{"empty quoted", `K=""`, ""},
		{"inline comment after quote", `K="v" # note`, "v"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := parseEnvFileBytes([]byte(tc.src + "\nAFTER=ok\n"))
			if len(entries) != 2 || entries[0].Value != tc.want || entries[1].Key != "AFTER" {
				t.Fatalf("parse = %#v, want K=%q then AFTER", entries, tc.want)
			}
			env := map[string]string{"K": entries[0].Value, "AFTER": "ok"}
			first := renderDotenv(env, nil)
			again := map[string]string{}
			for _, e := range parseEnvFileBytes([]byte(first)) {
				again[e.Key] = e.Value
			}
			if again["K"] != tc.want {
				t.Fatalf("re-import K = %q, want %q (export %q)", again["K"], tc.want, first)
			}
			if second := renderDotenv(again, nil); second != first {
				t.Fatalf("export not byte-equal after re-import:\n%q\n%q", first, second)
			}
			plan := planEnvImport(map[string]string{}, entries, nil, false)
			printed := strings.Join(append(append([]string{}, plan.New...), plan.Changed...), ",")
			if printed != "K,AFTER" {
				t.Errorf("import plan should list key names only, got %q", printed)
			}
		})
	}
}
