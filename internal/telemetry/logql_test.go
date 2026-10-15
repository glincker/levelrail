package telemetry

import (
	"strings"
	"testing"
)

func TestLogQLParseAccepts(t *testing.T) {
	tests := []struct {
		name, src string
		labels    map[string]string
		line      string
		wantLabel bool
		wantLine  bool
	}{
		{"app eq", `{app="web"}`, map[string]string{"app": "web"}, "x", true, true},
		{"app eq other", `{app="web"}`, map[string]string{"app": "api"}, "x", false, true},
		{"regex anchored", `{app=~"we"}`, map[string]string{"app": "web"}, "x", false, true},
		{"regex alt", `{app=~"web|api"}`, map[string]string{"app": "api"}, "x", true, true},
		{"stream and level", `{app="web", stream="stderr", level!="debug"}`, map[string]string{"app": "web", "stream": "stderr", "level": "error"}, "x", true, true},
		{"trailing comma", `{app="web",}`, map[string]string{"app": "web"}, "x", true, true},
		{"contains", `{app="web"} |= "boom"`, map[string]string{"app": "web"}, "a boom b", true, true},
		{"contains miss", `{app="web"} |= "boom"`, map[string]string{"app": "web"}, "fine", true, false},
		{"not contains", `{app="web"} != "health"`, map[string]string{"app": "web"}, "GET /health", true, false},
		{"regex line", `{app="web"} |~ "err(or)?\\s+\\d+"`, map[string]string{"app": "web"}, "error 42", true, true},
		{"neg regex line", `{app="web"} !~ "^debug"`, map[string]string{"app": "web"}, "debug x", true, false},
		{"chain", "{app=\"web\"} |= \"a\" != \"b\" |~ `c.`", map[string]string{"app": "web"}, "a cd", true, true},
		{"backtick", "{app=`web`}", map[string]string{"app": "web"}, "x", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := ParseLogQL(tc.src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := q.MatchLabels(tc.labels); got != tc.wantLabel {
				t.Errorf("MatchLabels = %v, want %v", got, tc.wantLabel)
			}
			if got := q.MatchLine(tc.line); got != tc.wantLine {
				t.Errorf("MatchLine = %v, want %v", got, tc.wantLine)
			}
		})
	}
}

func TestLogQLParseRejects(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"empty", ``, "empty query"},
		{"metric", `rate({app="web"}[5m])`, "metric queries"},
		{"sum", `sum(rate({app="web"}[5m]))`, "metric queries"},
		{"json parser", `{app="web"} | json`, "not supported"},
		{"logfmt", `{app="web"} | logfmt | level="error"`, "not supported"},
		{"unwrap", `{app="web"} | unwrap bytes`, "not supported"},
		{"unknown label", `{app="web", pod="x"}`, "unsupported label"},
		{"no app", `{stream="stdout"}`, "app matcher"},
		{"app negative only", `{app!="web"}`, "app matcher"},
		{"empty selector", `{}`, "empty stream selector"},
		{"unterminated", `{app="web"`, "unterminated"},
		{"bad regex", `{app=~"("}`, "invalid regex"},
		{"bad line regex", `{app="web"} |~ "("`, "invalid line filter regex"},
		{"unquoted", `{app=web}`, "quoted string"},
		{"garbage tail", `{app="web"} nope`, "line filters"},
		{"no start", `app="web"`, "stream selector"},
		{"unterminated string", `{app="web} `, "unterminated"},
		{"too long", `{app="` + strings.Repeat("a", maxLogQLLen) + `"}`, "too long"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseLogQL(tc.src)
			if err == nil {
				t.Fatalf("expected error for %q", tc.src)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
		})
	}
}
