package dockerlint

import (
	"strings"
	"testing"
)

func ruleSet(r Result) map[string]bool {
	m := map[string]bool{}
	for _, f := range r.Findings {
		m[f.Rule] = true
	}
	return m
}

func TestLint(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		ignore    bool
		want      []string
		notWant   []string
		wantFixed string
	}{
		{
			name:      "bare node image",
			src:       "FROM node\nWORKDIR /app\nRUN npm run build\nCMD [\"node\", \"x.js\"]\n",
			want:      []string{RuleFloatingTag, RuleRootUser, RuleHealthcheck, RuleMultiStage, RuleDockerignor},
			wantFixed: "USER 65532:65532\nCMD",
		},
		{
			name:    "hardened multi stage",
			src:     "FROM golang:1.22 AS build\nRUN go build -o /app\nFROM gcr.io/distroless/static:nonroot\nCOPY --from=build /app /app\nUSER 65532\nHEALTHCHECK CMD [\"/app\", \"-ping\"]\nENTRYPOINT [\"/app\"]\n",
			ignore:  true,
			notWant: []string{RuleRootUser, RuleHealthcheck, RuleMultiStage, RuleDockerignor, RuleFloatingTag},
		},
		{
			name:    "stage reference is not floating",
			src:     "FROM alpine:3.20 AS base\nFROM base\nUSER app\n",
			ignore:  true,
			notWant: []string{RuleFloatingTag, RuleRootUser},
		},
		{
			name:      "explicit root user without entrypoint appends",
			src:       "FROM alpine:3.20\nUSER root\n",
			ignore:    true,
			want:      []string{RuleRootUser},
			wantFixed: "USER root\nUSER 65532:65532\n",
		},
		{
			name:    "continuation lines and digest pin",
			src:     "FROM alpine@sha256:abc\nRUN apk add \\\n  curl\nUSER 1000\n",
			ignore:  true,
			notWant: []string{RuleFloatingTag, RuleRootUser},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := Lint(tc.src, tc.ignore)
			got := ruleSet(res)
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("missing rule %s in %+v", w, res.Findings)
				}
			}
			for _, n := range tc.notWant {
				if got[n] {
					t.Errorf("unexpected rule %s", n)
				}
			}
			if tc.wantFixed != "" && !strings.Contains(res.FixedDockerfile, tc.wantFixed) {
				t.Errorf("fixed dockerfile missing %q:\n%s", tc.wantFixed, res.FixedDockerfile)
			}
		})
	}
}

func TestLintEmpty(t *testing.T) {
	res := Lint("", true)
	if len(res.Findings) != 0 || res.Findings == nil {
		t.Fatalf("empty input should give an empty non-nil list, got %+v", res.Findings)
	}
}
