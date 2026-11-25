package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// commandSchema is the machine-readable contract of one command.
type commandSchema struct {
	Command     string      `json:"command"`
	Args        []schemaArg `json:"args,omitempty"`
	Flags       []docsFlag  `json:"flags,omitempty"`
	Subcommands []string    `json:"subcommands,omitempty"`
	JSONOutput  bool        `json:"json_output"`
	Example     string      `json:"example,omitempty"`
}

type schemaArg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

type cliSchemaDoc struct {
	GlobalFlags []string        `json:"global_flags"`
	Commands    []commandSchema `json:"commands"`
}

func schemaUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s schema [command [subcommand...]]

Prints JSON describing the command (or every command with no argument):
its arguments, flags with type and default, subcommands, whether --json is
supported, and one example. Offline, and built from the same help text as
%[1]s docs, so it never drifts from the real flags.
`, prog)
}

// schemaArgs reads <required> and [optional] positionals from the usage line.
func schemaArgs(prog string, path []string, help string) []schemaArg {
	line := firstUsageLine(prog, strings.Join(path, " "), help)
	rest := strings.TrimSpace(strings.TrimPrefix(line, prog+" "+strings.Join(path, " ")))
	var out []schemaArg
	for _, tok := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(tok, "<"):
			out = append(out, schemaArg{Name: strings.Trim(tok, "<>.,"), Required: true})
		case strings.HasPrefix(tok, "[") && !strings.HasPrefix(tok, "[-") && tok != "[flags]" && tok != "[options]":
			out = append(out, schemaArg{Name: strings.Trim(tok, "[].,")})
		}
	}
	return out
}

func buildSchema(prog string, n docsNode) commandSchema {
	hasJSON := strings.Contains(n.Help, "--json")
	for _, f := range n.Flags {
		if f.Name == "json" {
			hasJSON = true
		}
	}
	return commandSchema{
		Command:     strings.Join(n.Path, " "),
		Args:        schemaArgs(prog, n.Path, n.Help),
		Flags:       n.Flags,
		Subcommands: n.Children,
		JSONOutput:  hasJSON,
		Example:     n.Example,
	}
}

func runSchema(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" schema", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, schemaUsage(prog)) }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	want := strings.Join(fs.Args(), " ")
	doc := cliSchemaDoc{GlobalFlags: globalFlags, Commands: []commandSchema{}}
	for _, n := range collectDocsOpt(true) {
		if want == "" || strings.Join(n.Path, " ") == want {
			doc.Commands = append(doc.Commands, buildSchema(prog, n))
		}
	}
	if want != "" && len(doc.Commands) == 0 {
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n", want)
		return exitUsage
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
	return exitOK
}
