package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

// appTune holds the values init lets a person adjust before app.yaml is written.
type appTune struct {
	Name, Health, Domain string
	Port                 int
	Deploy               bool
}

func (t appTune) applyToStack(st *stackdetect.Stack) {
	if t.Name != "" {
		st.Name = t.Name
	}
	if t.Port > 0 {
		st.Port = t.Port
	}
}

// applyToYAML edits the generated spec in place: the readiness path and a domains list.
func (t appTune) applyToYAML(body string) string {
	if t.Health != "" && strings.HasPrefix(t.Health, "/") {
		body = strings.Replace(body, "readiness: { path: /,", "readiness: { path: "+t.Health+",", 1)
	}
	if t.Domain != "" {
		line := "    domains:\n      - " + t.Domain + "\n"
		if i := strings.Index(body, "    port:"); i >= 0 {
			body = body[:i] + line + body[i:]
		} else {
			body += line
		}
	}
	return body
}

func askLine(r *bufio.Reader, w io.Writer, label, def string) string {
	if def != "" {
		_, _ = fmt.Fprintf(w, "%s [%s]: ", label, def)
	} else {
		_, _ = fmt.Fprintf(w, "%s (blank for none): ", label)
	}
	line, _ := r.ReadString('\n')
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

func askYesNo(r *bufio.Reader, w io.Writer, q string) bool {
	_, _ = fmt.Fprintf(w, "%s [y/N] ", q)
	line, _ := r.ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}

// promptTune asks only for fields no flag already set, with the detected value as the default.
func promptTune(r *bufio.Reader, w io.Writer, st stackdetect.Stack, t appTune) appTune {
	if t.Name == "" {
		t.Name = askLine(r, w, "App name", st.Name)
	}
	if t.Port == 0 && st.Build != stackdetect.BuildStatic && st.Build != stackdetect.BuildCompose {
		def := ""
		if st.Port > 0 {
			def = strconv.Itoa(st.Port)
		}
		if n, err := strconv.Atoi(askLine(r, w, "Port", def)); err == nil && n > 0 && n < 65536 {
			t.Port = n
		}
	}
	if t.Health == "" && st.Build != stackdetect.BuildStatic && st.Build != stackdetect.BuildCompose {
		t.Health = askLine(r, w, "Health check path", "/")
	}
	if t.Domain == "" {
		t.Domain = askLine(r, w, "Domain", "")
	}
	return t
}

// deployAfterInit hands off to up in the project directory, which infers the rest.
func deployAfterInit(prog, dir, apiURL string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if dir != "." && dir != "" {
		prev, err := os.Getwd()
		if err != nil || os.Chdir(dir) != nil {
			_, _ = fmt.Fprintf(stderr, "Run \"%s up\" in %s to deploy.\n", prog, dir)
			return exitOK
		}
		defer func() { _ = os.Chdir(prev) }()
	}
	var args []string
	if apiURL != "" {
		args = append(args, "--api-url", apiURL)
	}
	return runUp(prog, args, stdout, stderr, lookupEnv, stdinSource)
}
