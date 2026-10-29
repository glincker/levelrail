package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// dryRunFlag keeps --dry-run a plain switch while also taking --dry-run=server.
type dryRunFlag struct {
	on   *bool
	mode string
}

func (d *dryRunFlag) String() string {
	if d.on == nil || !*d.on {
		return "false"
	}
	return d.mode
}

func (d *dryRunFlag) Set(v string) error {
	switch v {
	case "true", "client", "server":
		*d.on, d.mode = true, v
	case "false", "none":
		*d.on, d.mode = false, ""
	default:
		return fmt.Errorf("--dry-run takes server, client or no value, not %q", v)
	}
	return nil
}

func (d *dryRunFlag) IsBoolFlag() bool { return true }

type serverSideFlags struct {
	on      *bool
	manager *string
	force   *bool
}

// ssaPathFor maps a manifest kind to its v2 route; only kinds with a write path apply.
func ssaPathFor(kind string) (string, error) {
	switch strings.ToLower(kind) {
	case "app":
		return "apps", nil
	case "database":
		return "databases", nil
	}
	return "", newValidationError("--server-side applies App and Database documents, not %q", kind)
}

func defaultFieldManager(prog string) string {
	return filepath.Base(prog)
}

func runServerSideApply(c cliCall, prog string, af *applyFlags, ss serverSideFlags) int {
	if len(af.files) == 0 {
		return iacFail(c, newValidationError("pass at least one -f file, directory or -"))
	}
	files, err := readIaCFiles(af.files)
	if err != nil {
		return iacFail(c, err)
	}
	manager := *ss.manager
	if manager == "" {
		manager = defaultFieldManager(prog)
	}
	opts := apiclient.V2ApplyOptions{FieldManager: manager, Force: *ss.force, DryRun: *af.dryRun}
	ctx := context.Background()
	var results []apiclient.V2ApplyResult
	for _, f := range files {
		dec := yaml.NewDecoder(bytes.NewReader([]byte(f.Content)))
		for {
			var doc map[string]any
			err := dec.Decode(&doc)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return iacFail(c, fmt.Errorf("%s: %w", f.Name, err))
			}
			if len(doc) == 0 {
				continue
			}
			kind, _ := doc["kind"].(string)
			path, err := ssaPathFor(kind)
			if err != nil {
				return iacFail(c, fmt.Errorf("%s: %w", f.Name, err))
			}
			meta, _ := doc["metadata"].(map[string]any)
			name, _ := meta["name"].(string)
			if name == "" {
				return iacFail(c, newValidationError("%s: metadata.name is required", f.Name))
			}
			res, err := c.client.ApplyV2(ctx, path, name, doc, opts)
			if err != nil {
				return iacFail(c, fmt.Errorf("apply %s %q: %w", strings.ToLower(kind), name, err))
			}
			results = append(results, res)
			if !c.jsonOut {
				suffix := "serverside-applied"
				if res.DryRun {
					suffix += " (server dry run)"
				}
				_, _ = fmt.Fprintf(c.stdout, "%s/%s %s\n", strings.ToLower(kind), name, suffix)
			}
		}
	}
	if c.jsonOut {
		return c.render(results, func() {})
	}
	return exitOK
}

// specChanges is what edit sends: changed leaves, and null for every removed field,
// so untouched fields (and their hidden env values) stay as the server has them.
func specChanges(orig, edited map[string]any) map[string]any {
	out := map[string]any{}
	for k, ev := range edited {
		ov, had := orig[k]
		om, oIsMap := ov.(map[string]any)
		em, eIsMap := ev.(map[string]any)
		switch {
		case had && oIsMap && eIsMap && len(em) > 0:
			if sub := specChanges(om, em); len(sub) > 0 {
				out[k] = sub
			}
		case !had || !reflect.DeepEqual(ov, ev):
			out[k] = ev
		}
	}
	for k := range orig {
		if _, kept := edited[k]; !kept {
			out[k] = nil
		}
	}
	return out
}

func manifestSpec(raw []byte) (map[string]any, error) {
	var doc struct {
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Spec == nil {
		doc.Spec = map[string]any{}
	}
	return doc.Spec, nil
}
