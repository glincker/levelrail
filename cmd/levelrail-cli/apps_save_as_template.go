package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsSaveAsTemplate implements "apps save-as-template <name>": POST
// /api/v1/apps/{name}/save-as-template. No secret, database, or
// vault-backed env value is ever captured, only the key name.
func runAppsSaveAsTemplate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps save-as-template", "print the saved template as JSON to stdout and nothing else", stderr)
	var templateName, description string
	fs.StringVar(&templateName, "template-name", "", "name for the saved template (default: the app's own name)")
	fs.StringVar(&description, "description", "", "description for the saved template")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsSaveAsTemplateUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "apps save-as-template", "app name")
	if !ok {
		return exitUsage
	}

	if templateName == "" {
		templateName = name
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.SaveAppAsTemplate(context.Background(), name, apiclient.SaveAppAsTemplateRequest{
		Name:        templateName,
		Description: description,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save app %q as template: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() { printCustomTemplateDetailHuman(stdout, result) })
}

func printCustomTemplateDetailHuman(out io.Writer, t apiclient.CustomTemplateDetail) {
	_, _ = fmt.Fprintf(out, "id:                     %s\n", t.ID)
	_, _ = fmt.Fprintf(out, "name:                   %s\n", t.Name)
	if t.Description != "" {
		_, _ = fmt.Fprintf(out, "description:            %s\n", t.Description)
	}
	if t.SourceApp != "" {
		_, _ = fmt.Fprintf(out, "source_app:             %s\n", t.SourceApp)
	}
	_, _ = fmt.Fprintf(out, "requires_configuration: %t\n", t.RequiresConfiguration)
	if len(t.RequiredEnvKeys) > 0 {
		_, _ = fmt.Fprintln(out, "required_env_keys (never a value, only the key name):")
		for _, key := range t.RequiredEnvKeys {
			_, _ = fmt.Fprintf(out, "  - %s\n", key)
		}
	}
}

func appsSaveAsTemplateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps save-as-template <name> [--template-name NAME] [--description TEXT] [flags]

Derives a compose.yaml from <name>'s current image, ports, env, volumes,
command, and depends_on, and saves it as a reusable, operator-defined
template. Secret, database, and vault-backed env values are never
captured, only their key name: deploying the saved template will ask
for a real value first, the same way a built-in catalog entry that
needs configuration already does. See "templates list --custom" and
"templates deploy".

Flags:
  --template-name string   name for the saved template (default: <name>)
  --description string     description for the saved template
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the saved template as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
