package appwizard

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/importplan"
)

// Runtime identifiers, also the suffix of the memory override env var.
const (
	runtimeNode    = "node"
	runtimePython  = "python"
	runtimeGo      = "go"
	runtimeRuby    = "ruby"
	runtimePHP     = "php"
	runtimeElixir  = "elixir"
	runtimeJava    = "java"
	runtimeStatic  = "static"
	runtimeUnknown = "unknown"
)

const mib = 1 << 20

// defaultMemoryMB is the suggested limit per runtime, overridable with
// APP_WIZARD_MEMORY_MB_<RUNTIME>.
var defaultMemoryMB = map[string]int{
	runtimeNode: 512, runtimePython: 512, runtimeGo: 256, runtimeRuby: 1024,
	runtimePHP: 512, runtimeElixir: 512, runtimeJava: 1024, runtimeStatic: 128, runtimeUnknown: 512,
}

var (
	healthcheckURLRe = regexp.MustCompile(`(?im)^\s*HEALTHCHECK\b.*?https?://[^/\s"'\\]+(/[^\s"'\\]*)`)
	portFlagRe       = regexp.MustCompile(`(?:--port[= ]|-p[= ]|\bPORT=)(\d{2,5})\b`)
)

type reader func(rel string) (string, bool)

// detectStack names the runtime and framework from manifest files under the
// app directory, with the file that decided it as evidence.
func detectStack(read reader) (Stack, importplan.StackGuess) {
	if text, ok := read("Gemfile"); ok {
		low := strings.ToLower(text)
		switch {
		case strings.Contains(low, "rails"):
			return Stack{Runtime: runtimeRuby, Framework: "Rails", Evidence: "Gemfile lists rails"},
				importplan.StackGuess{Name: "Rails", Method: importplan.BuildRailpack, Port: 3000, Health: "/up", Why: "Rails defaults"}
		case strings.Contains(low, "sinatra"):
			return Stack{Runtime: runtimeRuby, Framework: "Sinatra", Evidence: "Gemfile lists sinatra"},
				importplan.StackGuess{Name: "Sinatra", Method: importplan.BuildRailpack, Port: 4567, Health: "/"}
		}
		return Stack{Runtime: runtimeRuby, Framework: "Ruby", Evidence: "Gemfile found"},
			importplan.StackGuess{Name: "Ruby", Method: importplan.BuildRailpack, Port: 3000, Health: "/"}
	}
	if text, ok := read("composer.json"); ok {
		low := strings.ToLower(text)
		name := "PHP"
		switch {
		case strings.Contains(low, "laravel/framework"):
			name = "Laravel"
		case strings.Contains(low, "symfony/"):
			name = "Symfony"
		}
		return Stack{Runtime: runtimePHP, Framework: name, Evidence: "composer.json found"},
			importplan.StackGuess{Name: name, Method: importplan.BuildRailpack, Port: 8080, Health: "/"}
	}
	if text, ok := read("mix.exs"); ok && strings.Contains(strings.ToLower(text), "phoenix") {
		return Stack{Runtime: runtimeElixir, Framework: "Phoenix", Evidence: "mix.exs lists phoenix"},
			importplan.StackGuess{Name: "Phoenix", Method: importplan.BuildRailpack, Port: 4000, Health: "/"}
	}
	g, ok := importplan.GuessStack(func(p string) (string, bool, error) {
		t, found := read(p)
		return t, found, nil
	})
	if !ok {
		return Stack{Runtime: runtimeUnknown, Evidence: "no manifest file recognised"}, importplan.StackGuess{Method: importplan.BuildUnknown}
	}
	rt := runtimeOf(g)
	return Stack{Runtime: rt, Framework: g.Name, Evidence: g.Why}, g
}

func runtimeOf(g importplan.StackGuess) string {
	switch {
	case g.Method == importplan.BuildStatic:
		return runtimeStatic
	case g.Name == "Go":
		return runtimeGo
	case g.Name == "Java":
		return runtimeJava
	case g.Name == "Django" || g.Name == "FastAPI" || g.Name == "Flask" || g.Name == "Python":
		return runtimePython
	default:
		return runtimeNode
	}
}

// memorySuggestion picks a limit by runtime, bumping Next.js which needs
// headroom at start, and honouring the env override.
func memorySuggestion(st Stack) Memory {
	mb := defaultMemoryMB[st.Runtime]
	if mb == 0 {
		mb = defaultMemoryMB[runtimeUnknown]
	}
	if st.Framework == "Next.js" {
		mb = 768
	}
	key := envMemoryPrefix + strings.ToUpper(st.Runtime)
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		mb = v
	}
	return Memory{Bytes: int64(mb) * mib, Evidence: fmt.Sprintf("%d MiB is a safe start for a %s app; raise it if the app is OOM-killed", mb, runtimeLabel(st))}
}

func runtimeLabel(st Stack) string {
	if st.Framework != "" {
		return st.Framework
	}
	return st.Runtime
}

// portFromScripts looks for an explicit port in package.json start-like scripts.
func portFromScripts(pkg string) (int, string) {
	for _, key := range []string{`"start"`, `"dev"`, `"serve"`, `"preview"`} {
		i := strings.Index(pkg, key)
		if i < 0 {
			continue
		}
		line := pkg[i:]
		if nl := strings.IndexByte(line, '\n'); nl > 0 {
			line = line[:nl]
		}
		if m := portFlagRe.FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n < 65536 {
				return n, "package.json " + strings.Trim(key, `"`) + " script sets port " + m[1]
			}
		}
	}
	return 0, ""
}

// healthFromDockerfile returns the path of an HTTP HEALTHCHECK, if any.
func healthFromDockerfile(df string) string {
	if m := healthcheckURLRe.FindStringSubmatch(df); m != nil {
		return m[1]
	}
	return ""
}
