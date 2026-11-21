package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// doctorLabel keeps the status column plain text, FAIL is upper case so it stands out without symbols.
func doctorLabel(status string) string {
	if status == "fail" {
		return "FAIL"
	}
	return status
}

func printDoctor(out io.Writer, report cloudRow) {
	steps, _ := report["steps"].([]any)
	for _, raw := range steps {
		s, _ := raw.(map[string]any)
		_, _ = fmt.Fprintf(out, "%-4s %s: %v\n", doctorLabel(fmt.Sprint(s["status"])), s["title"], s["detail"])
		if fix := fmt.Sprint(s["fix"]); s["fix"] != nil && fix != "" && s["status"] != "ok" {
			_, _ = fmt.Fprintf(out, "     fix: %s\n", fix)
		}
	}
	if ready, _ := report["ready"].(bool); ready {
		_, _ = fmt.Fprintln(out, "ready: no step failed")
	} else {
		_, _ = fmt.Fprintln(out, "not ready: fix the FAIL steps above")
	}
}

func runCloudDoctor(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var account string
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&account, "account", "", "check one cloud account; without it the control plane preflight runs")
	}
	return cloudCall(prog, "cloud doctor", args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
		path := "/doctor/preflight"
		if id := strings.TrimSpace(account); id != "" {
			path = "/" + url.PathEscape(id) + "/doctor"
		}
		var out cloudRow
		err := c.CloudRequest(context.Background(), "GET", path, nil, &out)
		return out, func() { printDoctor(stdout, out) }, err
	})
}
