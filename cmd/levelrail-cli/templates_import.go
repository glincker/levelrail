package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runTemplatesImport implements "templates import <file>": POST /api/v1/templates/custom.
func runTemplatesImport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var id, name, description string
	var vars stringListFlag
	cmd, code, ok := parseComposeCmd(prog, "templates import", "templates import <file | -> [--id ID] [--name NAME] [--description TEXT] [--var NAME]...", args, 1, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&id, "id", "", "template id (default: the file name without its extension)")
		fs.StringVar(&name, "name", "", "display name (default: the id)")
		fs.StringVar(&description, "description", "", "description")
		fs.Var(&vars, "var", "placeholder name filled in at deploy time (repeatable)")
	})
	if !ok {
		return code
	}
	body, err := readComposeFile(cmd.args[0])
	if err != nil {
		return reportError(stdout, stderr, cmd.json, err)
	}
	if id == "" {
		base := filepath.Base(cmd.args[0])
		id = strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
	}
	if name == "" {
		name = id
	}
	res, err := cmd.client.ImportCustomTemplate(context.Background(), apiclient.ImportCustomTemplateRequest{
		ID: id, Name: name, Description: description, ComposeYAML: string(body), Variables: vars,
	})
	if err != nil {
		return reportError(stdout, stderr, cmd.json, fmt.Errorf("import template %q: %w", id, err))
	}
	return renderCompose(stdout, stderr, cmd, res, func() {
		_, _ = fmt.Fprintf(stdout, "id:                     %s\nname:                   %s\nrequires_configuration: %t\n", res.ID, res.Name, res.RequiresConfiguration)
		for _, k := range res.RequiredEnvKeys {
			_, _ = fmt.Fprintf(stdout, "  needs %s\n", k)
		}
	})
}
