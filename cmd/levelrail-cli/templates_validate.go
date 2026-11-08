package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// runTemplatesValidate checks a community submission offline, with the same
// rules CI applies to a template pull request.
func runTemplatesValidate(prog string, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates validate <template.yaml | directory>\n\nChecks a template submission offline against the catalog rules. The compose\nfile is read from the same directory (compose.yaml unless template.yaml names another).\n", prog)
		return exitUsage
	}
	tplPath := args[0]
	if info, err := os.Stat(tplPath); err == nil && info.IsDir() {
		tplPath = filepath.Join(tplPath, "template.yaml")
	}
	tplBody, err := os.ReadFile(tplPath) //nolint:gosec // operator-chosen submission path
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: read template: %v\n", prog, err)
		return exitUsage
	}
	var head struct {
		ComposeFile string `yaml:"compose_file"`
	}
	if err := yaml.Unmarshal(tplBody, &head); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: template.yaml: %v\n", prog, err)
		return exitUsage
	}
	composeName := head.ComposeFile
	if composeName == "" {
		composeName = "compose.yaml"
	}
	composeBody, err := os.ReadFile(filepath.Join(filepath.Dir(tplPath), filepath.Base(composeName))) //nolint:gosec // sibling of the submission file
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: read compose file: %v\n", prog, err)
		return exitUsage
	}

	m, err := catalog.CheckSubmission(tplBody, composeBody)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "invalid: %s\n", m.ID)
		for _, line := range strings.Split(err.Error(), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				_, _ = fmt.Fprintf(stdout, "  %s\n", line)
			}
		}
		return exitValidation
	}
	_, _ = fmt.Fprintf(stdout, "ok: %s (%s)\n", m.ID, m.Name)
	return exitOK
}
