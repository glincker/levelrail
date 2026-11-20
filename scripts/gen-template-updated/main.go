// Command gen-template-updated writes internal/catalog/template_updated.json:
// the last commit date of each template directory, read from git history.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var roots = []string{"internal/catalog/manifests/", "registry/templates/"}

func main() {
	out, err := exec.CommandContext(context.Background(), "git", "log", "--name-only", "--format=@@%cs", "--", "internal/catalog/manifests", "registry/templates").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "git log:", err)
		os.Exit(1)
	}
	dates := map[string]string{}
	date := ""
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "@@") {
			date = strings.TrimPrefix(line, "@@")
			continue
		}
		for _, r := range roots {
			rest, ok := strings.CutPrefix(line, r)
			if !ok {
				continue
			}
			id, _, nested := strings.Cut(rest, "/")
			if _, seen := dates[id]; !seen && id != "" && nested {
				dates[id] = date
			}
		}
	}
	b, err := json.MarshalIndent(dates, "", " ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile("internal/catalog/template_updated.json", append(b, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
