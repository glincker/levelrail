package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func getUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s get <kind> [name] [-o table|wide|json|yaml|name] [-l SELECTOR] [--limit N] [flags]

Lists a resource kind (app, database, node) or shows one by name, the way
kubectl get does. Run "%[1]s api-resources" for the document kinds.

Flags:
  -o, --output string   table, wide (extra columns), json, yaml, name, text
  -l, --selector string label selector, for example env=prod,tier!=db,canary
  --limit N             page size for a list (default: the server default)
  --json                print the result as JSON and nothing else
  --query string        JMESPath expression applied to the result
%[2]s`, prog, commonFlagsHelp)
}

func describeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s describe <kind> <name> [--events N] [flags]

Prints one resource with its status, conditions and recent events.

Flags:
  --events N      how many recent events to show for an app (default 10)
  --json          print the result as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

func runGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var selector, selShort *string
	var limit *int
	c, code, ok := parseCLICall(prog, "get", getUsage(prog), expandShortFlags(args), stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		selector = fs.String("selector", "", "label selector")
		selShort = fs.String("l", "", "label selector (short)")
		limit = fs.Int("limit", 0, "page size for a list")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return c.invalid("get needs a kind and an optional name, for example: get apps")
	}
	kind, err := resolveResourceKind(rest[0])
	if err != nil {
		return c.fail(err)
	}
	sel := *selector
	if sel == "" {
		sel = *selShort
	}
	ctx := context.Background()
	wide := c.of.Format == outputWide
	if len(rest) == 2 {
		obj, err := c.client.GetV2(ctx, kind.Path, rest[1])
		if err != nil {
			return c.fail(fmt.Errorf("get %s %q: %w", kind.Singular, rest[1], err))
		}
		return c.renderObjects(kind, []apiclient.V2Object{obj}, obj, wide)
	}
	list, err := c.client.ListV2(ctx, kind.Path, sel, *limit)
	if err != nil {
		return c.fail(fmt.Errorf("list %s: %w", kind.Path, err))
	}
	return c.renderObjects(kind, list.Items, list, wide)
}

func (c cliCall) renderObjects(kind resourceKind, items []apiclient.V2Object, data any, wide bool) int {
	if c.of.Format == outputName && c.of.Query == "" {
		for _, o := range items {
			_, _ = fmt.Fprintf(c.stdout, "%s/%s\n", kind.Singular, o.Metadata.Name)
		}
		return exitOK
	}
	return c.render(data, func() { writeResourceTable(c.stdout, kind, items, wide) })
}

func columnsFor(kind resourceKind, wide bool) []string {
	cols := []string{"NAME", "PROJECT", "STATUS", "AGE"}
	if !wide {
		return cols
	}
	switch kind.Path {
	case "apps":
		return append(cols, "IMAGE", "PORT", "DOMAINS", "LABELS", "VERSION")
	case "databases":
		return append(cols, "ENGINE", "VERSION", "NODE", "LABELS")
	default:
		return append(cols, "PROVIDER", "REGION", "LABELS", "VERSION")
	}
}

func writeResourceTable(w io.Writer, kind resourceKind, items []apiclient.V2Object, wide bool) {
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "No resources found.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(columnsFor(kind, wide), "\t"))
	for _, o := range items {
		row := []string{o.Metadata.Name, dash(o.Metadata.Namespace), objectStatus(o), objectAge(o)}
		if wide {
			row = append(row, wideCells(kind, o)...)
		}
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

func wideCells(kind resourceKind, o apiclient.V2Object) []string {
	lbl, ver := labelString(o.Metadata.Labels), dash(o.Metadata.ResourceVersion)
	switch kind.Path {
	case "apps":
		return []string{dash(str(o.Spec["image"])), dash(str(o.Spec["port"])), dash(joinAny(o.Spec["domains"])), lbl, ver}
	case "databases":
		return []string{dash(str(o.Spec["engine"])), dash(str(o.Spec["version"])), dash(firstNonEmpty(str(o.Spec["node_id"]), str(o.Spec["node"]))), lbl}
	default:
		return []string{dash(str(o.Spec["provider"])), dash(str(o.Spec["region"])), lbl, ver}
	}
}

func objectStatus(o apiclient.V2Object) string {
	for _, k := range []string{"label", "phase"} {
		if s := str(o.Status[k]); s != "" {
			return s
		}
	}
	return dash(firstNonEmpty(str(o.Spec["status"]), str(o.Spec["state"])))
}

func objectAge(o apiclient.V2Object) string {
	t, err := time.Parse(time.RFC3339, o.Metadata.CreationTimestamp)
	if err != nil {
		return "-"
	}
	return humanAge(time.Since(t))
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func labelString(l map[string]string) string {
	if len(l) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(l))
	for k, v := range l {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return fmt.Sprintf("%g", x)
	}
	return fmt.Sprint(v)
}

func joinAny(v any) string {
	list, _ := v.([]any)
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, str(e))
	}
	return strings.Join(parts, ",")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

type describeResult struct {
	Object apiclient.V2Object       `json:"object"`
	Events []apiclient.TimelineItem `json:"events,omitempty"`
}

func runDescribe(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var events *int
	c, code, ok := parseCLICall(prog, "describe", describeUsage(prog), expandShortFlags(args), stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		events = fs.Int("events", 10, "recent events to show")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 2 {
		return c.invalid("describe needs a kind and a name, for example: describe app web")
	}
	kind, err := resolveResourceKind(rest[0])
	if err != nil {
		return c.fail(err)
	}
	ctx := context.Background()
	obj, err := c.client.GetV2(ctx, kind.Path, rest[1])
	if err != nil {
		return c.fail(fmt.Errorf("describe %s %q: %w", kind.Singular, rest[1], err))
	}
	res := describeResult{Object: obj}
	if kind.Path == "apps" && *events > 0 {
		// Events are best effort: a missing timeline must not hide the resource itself.
		if tl, terr := c.client.GetAppTimeline(ctx, rest[1], *events, ""); terr == nil {
			res.Events = tl.Items
		}
	}
	return c.render(res, func() { printDescribe(c.stdout, kind, res) })
}

func printDescribe(w io.Writer, kind resourceKind, res describeResult) {
	o := res.Object
	_, _ = fmt.Fprintf(w, "Name:        %s\nKind:        %s\nProject:     %s\nStatus:      %s\nLabels:      %s\nVersion:     %s\nCreated:     %s\n",
		o.Metadata.Name, kind.Singular, dash(o.Metadata.Namespace), objectStatus(o), labelString(o.Metadata.Labels), dash(o.Metadata.ResourceVersion), dash(o.Metadata.CreationTimestamp))
	if len(o.Metadata.Annotations) > 0 {
		_, _ = fmt.Fprintf(w, "Annotations: %s\n", labelString(o.Metadata.Annotations))
	}
	_, _ = fmt.Fprintln(w, "Spec:")
	keys := make([]string, 0, len(o.Spec))
	for k := range o.Spec {
		if k != "conditions" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := compactValue(o.Spec[k]); v != "" {
			_, _ = fmt.Fprintf(w, "  %-18s %s\n", k+":", v)
		}
	}
	if conds, _ := o.Spec["conditions"].([]any); len(conds) > 0 {
		_, _ = fmt.Fprintln(w, "Conditions:")
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  TYPE\tSTATUS\tREASON\tMESSAGE")
		for _, c := range conds {
			m, _ := c.(map[string]any)
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", str(m["type"]), str(m["status"]), dash(str(m["reason"])), dash(str(m["message"])))
		}
		_ = tw.Flush()
	}
	if len(res.Events) > 0 {
		_, _ = fmt.Fprintln(w, "Events:")
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  AT\tKIND\tSTATUS\tMESSAGE")
		for _, e := range res.Events {
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", e.At, e.Kind, dash(e.Status), e.Title)
		}
		_ = tw.Flush()
	}
}

// compactValue keeps nested spec values on one line so describe stays scannable.
func compactValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []any:
		return joinAny(x)
	case map[string]any:
		parts := make([]string, 0, len(x))
		for k, e := range x {
			parts = append(parts, k+"="+str(e))
		}
		sort.Strings(parts)
		return strings.Join(parts, ",")
	}
	return str(v)
}
