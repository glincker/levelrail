package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const envStackFileMaxBytes = "LEVELRAIL_IMPORT_FILE_MAX_BYTES"

// portainerExport is the JSON shape of an exported stack: a name, the compose
// text and an env list whose values are never read.
type portainerExport struct {
	Name             string `json:"Name"`
	StackFileContent string `json:"StackFileContent"`
	Env              []struct {
		Name string `json:"name"`
	} `json:"Env"`
}

func stackFileUsage(prog, sub string) string {
	what := "a deploy.yml"
	switch sub {
	case "portainer":
		what = "a stack compose file or a stack export JSON"
	case "render":
		what = "a render.yaml Blueprint (services, databases, cron jobs; no variable values)"
	case "railway":
		what = "a railway.json or railway.toml (build and deploy settings, no variable values)"
	}
	return fmt.Sprintf(`Usage:
  %[1]s import %[2]s FILE [--stack NAME] [--project NAME] [--environment NAME] [--image-repo REPO] [--write DIR [--force]] [--json]

Reads %[3]s and prints reviewable Levelrail documents (App kinds in a Stack).
Dry run by default: nothing is written unless --write DIR is given. Fields with no
equivalent are errors or listed in the notes. Secret values are never read: each
becomes a secret name listed in SECRETS.md. --image-repo gives an app with no image
(railway) the image path REPO/APP:latest. Size limit: %[4]s (default %[5]d bytes).
`, prog, sub, what, envStackFileMaxBytes, defaultComposeBytes)
}

func runImportStackFile(prog, sub string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f composeRefFlags
	fset := flag.NewFlagSet(prog+" import "+sub, flag.ContinueOnError)
	fset.SetOutput(stderr)
	fset.Usage = func() { _, _ = fmt.Fprint(stderr, stackFileUsage(prog, sub)) }
	fset.StringVar(&f.stack, "stack", "", "stack name (default: derived from the source)")
	fset.StringVar(&f.project, "project", "", "project name (default: the stack name)")
	fset.StringVar(&f.environment, "environment", "", "environment name for the Stack document")
	var imageRepo string
	fset.StringVar(&imageRepo, "image-repo", "", "registry path for an app the source gives no image, as REPO/APP:latest")
	fset.StringVar(&f.write, "write", "", "write the documents into this directory instead of printing them")
	fset.BoolVar(&f.force, "force", false, "overwrite existing files in --write")
	fset.BoolVar(&f.jsonOut, "json", false, "print the result as JSON")
	// FILE may come first or after the flags, so unknown flags are reported by the parser.
	path, rest := "", args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		path, rest = args[0], args[1:]
	}
	if err := fset.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if path == "" {
		path = fset.Arg(0)
	}
	if path == "" {
		_, _ = fmt.Fprint(stderr, stackFileUsage(prog, sub))
		return exitUsage
	}
	data, err := readBoundedFile(path, int64(envInt(lookupEnv, envStackFileMaxBytes, defaultComposeBytes)))
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	var in platformimport.EmitInput
	fallback := stackNameFromDir(filepath.Dir(path))
	switch sub {
	case "portainer":
		in, fallback, err = portainerInput(data, path)
	case "render":
		in, err = platformimport.RenderBlueprintToEmit(data)
		fallback = stackNameFromDir(filepath.Dir(path))
	case "railway":
		in, err = platformimport.RailwayToEmit(data, fallback, filepath.Base(path))
	default:
		in, err = platformimport.DeployYMLToEmit(data)
		if len(in.Apps) > 0 {
			fallback = in.Apps[0].Name
		}
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("%v", err))
	}
	stack := f.stack
	if stack == "" {
		stack = fallback
	}
	if err := iac.ValidateStackName(stack); err != nil {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--stack: %v", err))
	}
	project := f.project
	if project == "" {
		project = stack
	}
	opts := platformimport.EmitOptions{Stack: stack, Project: project, Environment: f.environment, ImageRepo: imageRepo}
	files, err := platformimport.EmitStack(in, opts)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if err := validateEmitFiles(files); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("emitted documents do not validate: %w", err))
	}
	if f.write != "" {
		return finishEmit(prog, opts, f.write, files, f.force, f.jsonOut, stdout, stderr)
	}
	return printComposeRef(stdout, stderr, stack, files, in.Notes, f.jsonOut)
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, newValidationError("%s is not a regular file", path)
	}
	if fi.Size() > limit {
		return nil, newValidationError("%s is larger than %d bytes", path, limit)
	}
	return os.ReadFile(path) //nolint:gosec // the operator names the file
}

// portainerInput converts a stack export or a plain compose file. Image
// services only: anything compose strict parsing rejects is a loud error.
func portainerInput(data []byte, path string) (platformimport.EmitInput, string, error) {
	in := platformimport.EmitInput{Source: "portainer stack " + filepath.Base(path)}
	name := ""
	text := data
	var exp portainerExport
	if json.Unmarshal(data, &exp) == nil && exp.StackFileContent != "" {
		text, name = []byte(exp.StackFileContent), platformimport.SanitizeName(exp.Name)
		var names []string
		for _, e := range exp.Env {
			names = append(names, e.Name)
		}
		if len(names) > 0 {
			in.Notes = append(in.Notes, "stack env names, values not read: "+strings.Join(names, ", "))
		}
	}
	if name == "" {
		name = stackNameFromDir(filepath.Dir(path))
	}
	cf, err := compose.Parse(text)
	if err != nil {
		return in, name, fmt.Errorf("stack file does not parse: %w", err)
	}
	svcNames := make([]string, 0, len(cf.Services))
	for n := range cf.Services {
		svcNames = append(svcNames, n)
	}
	sort.Strings(svcNames)
	for _, n := range svcNames {
		svc := cf.Services[n]
		if svc.Image == "" {
			in.Notes = append(in.Notes, fmt.Sprintf("service %s has a build context, skipped: build and push an image, then add an App for it", n))
			continue
		}
		app := platformimport.EmitApp{Name: platformimport.SanitizeName(n), Image: svc.Image, PortlessOK: true}
		if len(svc.Ports) > 0 {
			app.Port = svc.Ports[0].ContainerPort
		}
		keys := make([]string, 0, len(svc.Environment))
		for k := range svc.Environment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			app.Secrets = append(app.Secrets, platformimport.EmitSecret{Name: k, Hint: "environment of service " + n + ", value not read"})
		}
		app.Notes = composeServiceNotes("stack file", n, svc)
		in.Apps = append(in.Apps, app)
	}
	if len(in.Apps) == 0 {
		return in, name, fmt.Errorf("no image based service to emit:\n  %s", strings.Join(in.Notes, "\n  "))
	}
	return in, name, nil
}
