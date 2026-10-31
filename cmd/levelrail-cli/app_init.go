package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func appsInitUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps init [--file app.yaml] [--name NAME] [--image IMAGE | --build TYPE] [--port N] [--domain HOST] [--database ENGINE] [--force] [--json]

Writes a starter app.yaml. Optional field names and hints come from the
embedded schema, so the file matches what the control plane accepts.
Runs offline and refuses to overwrite a file unless --force is set.

Flags:
  --file string       output path (default app.yaml)
  --name string       service name (default web)
  --image string      prebuilt image (omit to build from source)
  --build string      build type without an image: dockerfile, railpack or static (default dockerfile)
  --port int          container port (default 3000 unless --build static)
  --domain string     public host name
  --database string   add a managed database engine
  --force             overwrite an existing file
  --json              print the result as JSON to stdout, nothing else
`, prog)
}

type appInitResult struct {
	File string `json:"file"`
	Name string `json:"name"`
}

func runAppsInit(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" apps init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsInitUsage(prog)) }
	var o spec.ScaffoldOptions
	file := fs.String("file", "app.yaml", "output path")
	fs.StringVar(&o.Name, "name", "", "service name")
	fs.StringVar(&o.Image, "image", "", "prebuilt image")
	fs.StringVar(&o.Build, "build", "", "build type without an image")
	fs.IntVar(&o.Port, "port", 0, "container port")
	fs.StringVar(&o.Domain, "domain", "", "public host name")
	fs.StringVar(&o.DB, "database", "", "managed database engine")
	force := fs.Bool("force", false, "overwrite an existing file")
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprint(stderr, appsInitUsage(prog))
		return exitUsage
	}
	data, err := spec.Scaffold(o)
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, newValidationError("%v", err))
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if *force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(*file, flags, 0o600) //nolint:gosec // operator-supplied CLI flag
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return reportError(stdout, stderr, *jsonOut, newValidationError("%s already exists (use --force to overwrite)", *file))
		}
		return reportError(stdout, stderr, *jsonOut, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return reportError(stdout, stderr, *jsonOut, err)
	}
	if err := f.Close(); err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	name := o.Name
	if name == "" {
		name = "web"
	}
	if *jsonOut {
		if err := writeJSONValue(stdout, appInitResult{File: *file, Name: name}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\nnext: %s apps validate --file %s\n", *file, prog, *file)
	return exitOK
}
