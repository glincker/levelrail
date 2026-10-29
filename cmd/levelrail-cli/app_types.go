package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func appTypesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s app-types list
  %[1]s app-types apply -f FILE [--publish]
  %[1]s app-types check NAME -f SPEC_FILE [--version N] [--fail]

App types are admin-defined constraints an app spec is checked against. They need
LEVELRAIL_APPTYPE_ENABLED=on on the control plane. apply saves the draft from a YAML or
JSON document; --publish also freezes it as the next immutable version. check is a dry
run of a spec file (YAML or JSON) against a published version and changes nothing.
`, prog)
}

func runAppTypes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	subs := map[string]apiCmd{"list": appTypesListCommand(), "apply": appTypesApplyCommand(), "check": appTypesCheckCommand()}
	return dispatchSub(prog, "app-types", appTypesUsage, subs, args, stdout, stderr, lookupEnv)
}

// readJSONOrYAML loads a file and returns it as JSON bytes.
func readJSONOrYAML(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied CLI flag
	if err != nil {
		return nil, newValidationError("cannot read %s: %v", path, err)
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, newValidationError("%s is not valid YAML or JSON: %v", path, err)
	}
	return json.Marshal(v)
}

func appTypesListCommand() apiCmd {
	return apiCmd{
		label: "app-types list",
		usage: appTypesUsage,
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			list, err := c.ListAppTypes(ctx)
			return list, func(w io.Writer) { printAppTypes(w, list) }, err
		},
	}
}

func appTypesApplyCommand() apiCmd {
	var file string
	var publish bool
	return apiCmd{
		label: "app-types apply",
		usage: appTypesUsage,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&file, "f", "", "type document, YAML or JSON")
			fs.BoolVar(&publish, "publish", false, "freeze the saved draft as the next version")
		},
		run: func(ctx context.Context, c *Client, _ []string) (any, func(io.Writer), error) {
			if file == "" {
				return nil, nil, newValidationError("give the type document with -f FILE")
			}
			doc, err := readJSONOrYAML(file)
			if err != nil {
				return nil, nil, err
			}
			var head struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(doc, &head)
			if err := c.SaveAppType(ctx, doc); err != nil {
				return nil, nil, err
			}
			if !publish {
				return map[string]string{"name": head.Name, "saved": "draft"}, func(w io.Writer) {
					_, _ = fmt.Fprintf(w, "saved draft of %s (publish with --publish)\n", head.Name)
				}, nil
			}
			res, err := c.PublishAppType(ctx, head.Name)
			return res, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "published %s@%d (digest %.12s)\n", res.Name, res.Version, res.Digest)
			}, err
		},
	}
}

func appTypesCheckCommand() apiCmd {
	var file string
	var version int
	var failOnBlock bool
	return apiCmd{
		label: "app-types check",
		usage: appTypesUsage,
		args:  1,
		setup: func(fs *flag.FlagSet) {
			fs.StringVar(&file, "f", "", "app spec to check, YAML or JSON")
			fs.IntVar(&version, "version", 0, "published version to check against (default newest)")
			fs.BoolVar(&failOnBlock, "fail", false, "exit non-zero when a violation would block")
		},
		run: func(ctx context.Context, c *Client, pos []string) (any, func(io.Writer), error) {
			if file == "" {
				return nil, nil, newValidationError("give the app spec with -f FILE")
			}
			raw, err := readJSONOrYAML(file)
			if err != nil {
				return nil, nil, err
			}
			var spec map[string]any
			if err := json.Unmarshal(raw, &spec); err != nil {
				return nil, nil, newValidationError("%s must hold a mapping at the top level", file)
			}
			res, err := c.CheckAppType(ctx, pos[0], apiclient.AppTypeCheckRequest{Version: version, Spec: spec})
			if err == nil && failOnBlock && res.WouldFail {
				err = fmt.Errorf("app spec does not conform to %s@%d", res.Name, res.Version)
			}
			return res, func(w io.Writer) { printAppTypeCheck(w, res) }, err
		},
	}
}

func printAppTypes(out io.Writer, list []apiclient.AppTypeSummary) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no app types")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tPUBLISHED\tUPDATED BY")
	for _, t := range list {
		published := "draft only"
		if t.LatestVersion > 0 {
			published = fmt.Sprintf("v%d", t.LatestVersion)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, published, t.UpdatedBy)
	}
	_ = tw.Flush()
}

func printAppTypeCheck(out io.Writer, res apiclient.AppTypeCheckResult) {
	if len(res.Violations) == 0 {
		_, _ = fmt.Fprintf(out, "conforms to %s@%d\n", res.Name, res.Version)
		return
	}
	_, _ = fmt.Fprintf(out, "%d violations against %s@%d\n", len(res.Violations), res.Name, res.Version)
	for _, v := range res.Violations {
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\n  next: %s\n", v.Mode, v.Code, v.Message, v.NextAction)
	}
}
