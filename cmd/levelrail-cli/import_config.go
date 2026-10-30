package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/configtranslate"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const (
	envImportWebPort  = "LEVELRAIL_IMPORT_WEB_PORT"
	envImportNodePort = "LEVELRAIL_IMPORT_NODE_PORT"
	// maxConfigFileBytes keeps a stray huge file from being slurped into memory.
	maxConfigFileBytes = 1 << 20
)

var importConfigFiles = []string{"Procfile", "app.json", "vercel.json"}

// importConfigOutput is the --json shape of "import config".
type importConfigOutput struct {
	Detected []configtranslate.Format  `json:"detected"`
	Spec     string                    `json:"spec"`
	Findings []configtranslate.Finding `json:"findings"`
	// NextSteps are the exact commands that deploy the proposal.
	NextSteps []string `json:"nextSteps"`
}

// runImportConfig implements "import config": an offline, read-only preview
// of a Procfile, app.json and vercel.json as an app spec. It needs no token.
func runImportConfig(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var out string
	var strict bool
	var jsonOut bool
	var outputFlag, query string
	fset := flag.NewFlagSet(prog+" import config", flag.ContinueOnError)
	fset.SetOutput(stderr)
	fset.Usage = func() { _, _ = fmt.Fprint(stderr, importConfigUsage(prog)) }
	fset.StringVar(&out, "out", "", "write the proposed app.yaml to this file instead of stdout")
	fset.BoolVar(&strict, "strict", false, "exit non-zero when any finding is a blocker")
	fset.BoolVar(&jsonOut, "json", false, "print the result as JSON to stdout and nothing else")
	fset.StringVar(&outputFlag, "output", "", "output format: json or table")
	fset.StringVar(&query, "query", "", "project the JSON result with a dotted path")
	var em importEmitFlags
	var services, dbVersion string
	bindImportEmitFlags(fset, &em, false)
	fset.StringVar(&services, "service", "", "with --emit stack: only these services, comma separated (like apps create --service)")
	fset.StringVar(&dbVersion, "db-version", "", "with --emit stack: engine versions for the proposed databases, e.g. postgres=16,redis=7.4")
	jsonOut, of, exitCode, ok := parseSessionFlags(fset, reorderArgsFlagsFirst(fset, args), &jsonOut, &outputFlag, &query, prog, stderr)
	if !ok {
		return exitCode
	}
	if fset.NArg() > 1 {
		return reportError(stdout, stderr, jsonOut, newValidationError("import config takes at most one directory"))
	}
	dir := "."
	if fset.NArg() == 1 {
		dir = fset.Arg(0)
	}
	opts, err := importConfigOptions(lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	files, err := readConfigFiles(dir)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	res := configtranslate.Translate(files, opts...)
	if len(res.Detected) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("no Procfile, app.json or vercel.json found in %s", dir))
	}
	if em.active() {
		em.out = out
		return runImportConfigEmit(prog, dir, res, em, services, dbVersion, jsonOut, stdout, stderr)
	}
	yamlBytes, err := appCreatableFragment(res.Fragment).YAML()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if res.Findings == nil {
		res.Findings = []configtranslate.Finding{}
	}
	if out != "" {
		if err := os.WriteFile(out, yamlBytes, 0o644); err != nil { //nolint:gosec // a spec file the operator asked for, meant to be read and committed
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("write %s: %w", out, err))
		}
	}
	specFile := "app.yaml"
	if out != "" {
		specFile = out
	}
	steps := importConfigSteps(res.Fragment, prog, specFile)
	if of.Format == outputJSON || of.Query != "" {
		data := importConfigOutput{Detected: res.Detected, Spec: string(yamlBytes), Findings: res.Findings, NextSteps: steps}
		if err := renderResult(stdout, of.Format, of.Query, data, func() {}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
	} else {
		if out == "" {
			_, _ = stdout.Write(yamlBytes)
		} else {
			_, _ = fmt.Fprintf(stderr, "wrote %s\n", out)
		}
		printConfigFindings(stderr, res.Findings)
		printNextSteps(stderr, steps)
	}
	if strict && hasBlocker(res.Findings) {
		_, _ = fmt.Fprintln(stderr, "blocker findings present and --strict is set")
		return exitValidation
	}
	return exitOK
}

// runImportConfigEmit writes the proposal as stack documents instead of one app.yaml.
func runImportConfigEmit(prog, dir string, res configtranslate.Result, em importEmitFlags, services, dbVersion string, jsonOut bool, stdout, stderr io.Writer) int {
	opts, err := em.options(stackNameFromDir(dir))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	dbv, err := platformimport.ParseDBVersions(dbVersion)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("--db-version: %v", err))
	}
	eo := configtranslate.EmitOptions{DBVersions: dbv}
	for _, s := range strings.Split(services, ",") {
		if s = strings.TrimSpace(s); s != "" {
			eo.Services = append(eo.Services, s)
		}
	}
	in, err := res.EmitInput(eo)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	files, err := platformimport.EmitStack(in, opts)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	return finishEmit(prog, opts, em.out, files, em.force, jsonOut, stdout, stderr)
}

func printNextSteps(w io.Writer, steps []string) {
	if len(steps) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "\nNEXT STEPS\n")
	for _, st := range steps {
		_, _ = fmt.Fprintf(w, "  %s\n", st)
	}
}

func hasBlocker(fs []configtranslate.Finding) bool {
	for _, f := range fs {
		if f.Severity == configtranslate.SeverityBlocker {
			return true
		}
	}
	return false
}

// printConfigFindings groups findings blocker first, then warn, then info.
func printConfigFindings(w io.Writer, fs []configtranslate.Finding) {
	if len(fs) == 0 {
		_, _ = fmt.Fprintln(w, "No findings.")
		return
	}
	for _, sev := range []configtranslate.Severity{configtranslate.SeverityBlocker, configtranslate.SeverityWarn, configtranslate.SeverityInfo} {
		var group []configtranslate.Finding
		for _, f := range fs {
			if f.Severity == sev {
				group = append(group, f)
			}
		}
		if len(group) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n%s (%d)\n", strings.ToUpper(string(sev)), len(group))
		for _, f := range group {
			subject := ""
			if f.Subject != "" {
				subject = " " + f.Subject
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s%s: %s\n", f.Code, f.Source, subject, f.Message)
			if f.Fix != "" {
				_, _ = fmt.Fprintf(w, "    fix: %s\n", f.Fix)
			}
		}
	}
}

// importConfigOptions reads the proposed default ports from the environment.
func importConfigOptions(lookupEnv func(string) (string, bool)) ([]configtranslate.Option, error) {
	var opts []configtranslate.Option
	for _, e := range []struct {
		name string
		opt  func(int) configtranslate.Option
	}{{envImportWebPort, configtranslate.WithWebPort}, {envImportNodePort, configtranslate.WithNodePort}} {
		raw, ok := lookupEnv(e.name)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > 65535 {
			return nil, newValidationError("%s must be a port between 1 and 65535, got %q", e.name, raw)
		}
		opts = append(opts, e.opt(n))
	}
	return opts, nil
}

func readConfigFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range importConfigFiles {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if info.IsDir() {
			continue
		}
		if info.Size() > maxConfigFileBytes {
			return nil, newValidationError("%s is larger than %d bytes", p, maxConfigFileBytes)
		}
		b, err := os.ReadFile(p) //nolint:gosec // the operator names the directory to read
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		files[name] = b
	}
	return files, nil
}

func importConfigUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import config [dir] [--out FILE] [--strict] [--json]
  %[1]s import config [dir] --emit stack --out DIR [--stack NAME] [--image-repo REPO] [--service S] [--db-version postgres=16]

Reads Procfile, app.json and vercel.json from dir (default: the current
directory) and prints a proposed app.yaml. Findings (what has no equivalent,
what to check) go to stderr, blockers first, each with a fix. Offline and
read-only: no token is needed and nothing is created anywhere.

Flags:
  --out FILE   write the proposed app.yaml to FILE instead of stdout
  --strict     exit 2 when any finding is a blocker (default: exit 0)
  --json       print {detected, spec, findings} as JSON to stdout and nothing else
  --emit stack write a Stack, Project, Database, App and Domain documents plus SECRETS.md
               (secret names only) and MIGRATION.md into the --out directory, for review in a pull request
  --service S  with --emit: only these services, comma separated (as apps create --service)
  --image-repo REPO  with --emit: registry path, apps are named REPO/SERVICE:latest (apply runs images only)
  --db-version E=V   with --emit: version per database engine (the config files name none)
  --stack, --project, --environment, --force   with --emit: names, and overwrite existing files
  --output, --query   as for every command

Environment:
  %[2]s   port proposed for Heroku web and worker services (default 8080)
  %[3]s  port proposed for a Vercel server service (default 3000)
`, prog, envImportWebPort, envImportNodePort)
}
