package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runPluginsRun(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins run", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	app := fs.String("app", "", "app the command is scoped to (for panels shown on an app page)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "plugins run", "a plugin name and a command", 2)
	if !ok {
		return exitUsage
	}
	name, id := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	p, err := client.GetPlugin(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins run %q: %w", name, err))
	}
	cmd, found := findPluginCommand(p, id)
	if !found {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugin %q declares no command %q (it declares: %s)", name, id, pluginCommandIDs(p)))
	}
	if cmd.Action != "" {
		res, err := client.PluginPanelAction(ctx, name, cmd.Panel, cmd.Action, *app)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins run %s %s: %w", name, id, err))
		}
		code := exitOK
		if !res.OK {
			code = exitCheckFailed
		}
		if rc := writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintln(stdout, actionLine(res)) }); rc != exitOK {
			return rc
		}
		return code
	}
	view, err := client.PluginPanelData(ctx, name, cmd.Panel, *app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins run %s %s: %w", name, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, view, func() { printPluginView(stdout, view) })
}

func findPluginCommand(p apiclient.Plugin, id string) (apiclient.PluginCommand, bool) {
	for _, c := range p.Commands {
		if c.ID == id {
			return c, true
		}
	}
	return apiclient.PluginCommand{}, false
}

func pluginCommandIDs(p apiclient.Plugin) string {
	ids := make([]string, 0, len(p.Commands))
	for _, c := range p.Commands {
		ids = append(ids, c.ID)
	}
	return joinOrNone(ids)
}

func actionLine(res apiclient.PluginActionResult) string {
	status := "ok"
	if !res.OK {
		status = "failed"
	}
	if res.Message == "" {
		return status
	}
	return status + ": " + res.Message
}

func printPluginView(out io.Writer, v apiclient.PluginPanelView) {
	for i, w := range v.Widgets {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}
		printPluginWidget(out, w)
	}
}

func printPluginWidget(out io.Writer, w apiclient.PluginWidgetView) {
	if w.Title != "" {
		_, _ = fmt.Fprintf(out, "%s\n", w.Title)
	}
	if w.Error != "" {
		_, _ = fmt.Fprintf(out, "  unavailable: %s\n", w.Error)
		return
	}
	switch w.Type {
	case "stat":
		_, _ = fmt.Fprintf(out, "  %s\n", strings.TrimSpace(w.Value+" "+w.Unit))
	case "status":
		_, _ = fmt.Fprintf(out, "  %s %s\n", strings.ToUpper(w.State), w.Text)
	case "markdown":
		_, _ = fmt.Fprintf(out, "%s\n", w.Text)
	case "list":
		for _, it := range w.Items {
			_, _ = fmt.Fprintf(out, "  - %s\n", it)
		}
	case "chart":
		for _, pt := range w.Points {
			_, _ = fmt.Fprintf(out, "  %s\t%g\n", pt.Label, pt.Value)
		}
	case "table":
		printPluginTable(out, w)
	}
}

func printPluginTable(out io.Writer, w apiclient.PluginWidgetView) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	heads := make([]string, 0, len(w.Columns))
	for _, c := range w.Columns {
		heads = append(heads, strings.ToUpper(c.Label))
	}
	_, _ = fmt.Fprintln(tw, strings.Join(heads, "\t"))
	for _, row := range w.Rows {
		_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}
