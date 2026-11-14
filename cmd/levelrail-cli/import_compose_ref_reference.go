package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// composeVarRe finds ${NAME} and ${NAME:?x} forms; a default (-, :-) means the file needs no value.
var composeVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)([^}]*)\}`)

var composeSecretNameRe = regexp.MustCompile(`(?i)pass|secret|token|key|credential|dsn`)

// refApp is one referencing App: the compose file stays where it is.
type refApp struct {
	name, rel string
	vars      []string
	data      []byte
}

type refDoc struct {
	Version  int                   `yaml:"version"`
	Kind     string                `yaml:"kind"`
	Metadata struct{ Name string } `yaml:"metadata"`
	Spec     refSpec               `yaml:"spec"`
}

type refSpec struct {
	Project     string     `yaml:"project,omitempty"`
	Environment string     `yaml:"environment,omitempty"`
	Compose     refCompose `yaml:"compose"`
}

type refCompose struct {
	Path     string            `yaml:"path"`
	Profiles []string          `yaml:"profiles,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
}

// composeRefRoot is where the documents live: compose paths are relative to it.
func composeRefRoot(scanDir, write string) string {
	if write != "" {
		return write
	}
	return scanDir
}

// referenceAppName names an App after the file, or its folder for generic file names.
func referenceAppName(rel, stack string, used map[string]bool) string {
	stem := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if composeFileRe.MatchString(path.Base(rel)) && (stem == "docker-compose" || stem == "compose") {
		if dir := path.Base(path.Dir(rel)); dir != "." && dir != "/" {
			stem = dir
		} else {
			stem = stack
		}
	}
	return uniqueComposeAppName(stem, rel, used)
}

func requiredComposeVars(data []byte) []string {
	seen := map[string]bool{}
	for _, m := range composeVarRe.FindAllSubmatch(data, -1) {
		if strings.HasPrefix(string(m[2]), "-") || strings.HasPrefix(string(m[2]), ":-") || strings.HasPrefix(string(m[2]), "+") || strings.HasPrefix(string(m[2]), ":+") {
			continue
		}
		seen[string(m[1])] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a refApp) env() map[string]string {
	if len(a.vars) == 0 {
		return nil
	}
	env := map[string]string{}
	for _, v := range a.vars {
		if composeSecretNameRe.MatchString(v) {
			env[v] = "${{ secrets." + v + " }}"
		} else {
			env[v] = "${{ env." + v + " }}"
		}
	}
	return env
}

// composeRefStrictNotes runs the strict parser the server uses and returns one note per refusal.
func composeRefStrictNotes(rel string, data []byte, vars []string) []string {
	declared := map[string]string{}
	for _, v := range vars {
		declared[v] = "set"
	}
	_, err := iac.StrictComposeCheck(data, rel, declared)
	var notes []string
	for _, is := range iac.ComposeStrictIssues(rel, err) {
		notes = append(notes, fmt.Sprintf("%s: %s: %s", is.File, is.Code, is.Message))
	}
	return notes
}

// buildReferenceFiles emits one App per compose file that strict mode accepts.
func buildReferenceFiles(root, scanDir string, scan composeScan, o platformimport.EmitOptions, profiles []string) ([]platformimport.EmitFile, []string, error) {
	notes := append([]string(nil), scan.Notes...)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	used := map[string]bool{}
	var apps []refApp
	attach := map[string][]byte{}
	for _, rel := range scan.Files {
		abs, err := filepath.Abs(filepath.Join(scanDir, filepath.FromSlash(rel)))
		if err != nil {
			return nil, nil, err
		}
		docRel, err := filepath.Rel(absRoot, abs)
		docRel = filepath.ToSlash(docRel)
		if err != nil || docRel == ".." || strings.HasPrefix(docRel, "../") {
			return nil, nil, newValidationError("%s is outside the document directory %s: write the documents into the folder that holds the compose files (--write), or move the stack folder", rel, root)
		}
		data, err := os.ReadFile(abs) //nolint:gosec // the operator names the scanned directory
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", rel, err)
		}
		vars := requiredComposeVars(data)
		if n := composeRefStrictNotes(docRel, data, vars); len(n) > 0 {
			notes = append(notes, n...)
			notes = append(notes, docRel+" is not emitted: strict mode would refuse it at plan time; fix the file or keep it out of the stack")
			continue
		}
		apps = append(apps, refApp{name: referenceAppName(docRel, o.Stack, used), rel: docRel, vars: vars, data: data})
		attach[docRel] = data
	}
	if len(apps) == 0 {
		return nil, notes, newValidationError("no compose file passes strict mode:\n  %s", strings.Join(notes, "\n  "))
	}
	base, err := platformimport.EmitStack(platformimport.EmitInput{Source: "compose references"}, o)
	if err != nil {
		return nil, nil, err
	}
	var files []platformimport.EmitFile
	for _, f := range base {
		if f.Name == platformimport.EmitStackFile || f.Name == platformimport.EmitProjectFile {
			files = append(files, f)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	for _, a := range apps {
		d := refDoc{Version: 1, Kind: "App"}
		d.Metadata.Name = a.name
		d.Spec = refSpec{Project: o.Project, Environment: o.Environment, Compose: refCompose{Path: a.rel, Profiles: profiles, Env: a.env()}}
		if err := enc.Encode(d); err != nil {
			return nil, nil, err
		}
		for _, v := range a.vars {
			if !composeSecretNameRe.MatchString(v) {
				notes = append(notes, fmt.Sprintf("%s reads ${%s}: pass it at plan and apply with --var %s=VALUE", a.rel, v, v))
			} else {
				notes = append(notes, fmt.Sprintf("%s reads ${%s}: pass it with --secret %s=ENVVAR", a.rel, v, v))
			}
		}
	}
	if err := enc.Close(); err != nil {
		return nil, nil, err
	}
	files = append(files, platformimport.EmitFile{Name: platformimport.EmitAppsFile, Content: buf.Bytes()})
	if err := validateReferenceFiles(files, attach); err != nil {
		return nil, nil, fmt.Errorf("emitted documents do not validate: %w", err)
	}
	notes = append(notes, "Compose files are referenced in place and never copied: the compose file is the single definition of the services.")
	return files, notes, nil
}

// validateReferenceFiles runs the real loader with the gate on, offline.
func validateReferenceFiles(files []platformimport.EmitFile, attach map[string][]byte) error {
	var srcs []iac.Source
	for _, f := range files {
		if strings.HasSuffix(f.Name, ".yaml") && f.Name != platformimport.EmitStackFile {
			srcs = append(srcs, iac.Source{Name: f.Name, Data: f.Content})
		}
	}
	iac.SetComposeSource(func() iac.ComposeLimits {
		return iac.ComposeLimits{Enabled: true, MaxFileBytes: 256 << 10, MaxFiles: 20, MaxTotalBytes: 2048 << 10}
	})
	vars := map[string]string{}
	for _, f := range files {
		for _, m := range envPlaceholderRe.FindAllSubmatch(f.Content, -1) {
			vars[string(m[1])] = "placeholder"
		}
	}
	docs, issues := iac.ParseDocuments(srcs)
	if len(issues) == 0 {
		_, issues = iac.Build(docs, iac.Options{Vars: vars, Attachments: attach})
	}
	if len(issues) > 0 {
		return fmt.Errorf("%v", issues)
	}
	return nil
}

var envPlaceholderRe = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// runComposeReference is the --reference mode of import compose-ref.
func runComposeReference(prog string, f composeRefFlags, dir string, scan composeScan, stdout, stderr io.Writer) int {
	stack := f.stack
	if stack == "" {
		stack = stackNameFromDir(dir)
	}
	if err := iac.ValidateStackName(stack); err != nil {
		return reportError(stdout, stderr, f.jsonOut, newValidationError("--stack: %v", err))
	}
	project := f.project
	if project == "" {
		project = stack
	}
	opts := platformimport.EmitOptions{Stack: stack, Project: project, Environment: f.environment}
	var profiles []string
	for _, p := range strings.Split(f.profiles, ",") {
		if p = strings.TrimSpace(p); p != "" {
			profiles = append(profiles, p)
		}
	}
	files, notes, err := buildReferenceFiles(composeRefRoot(dir, f.write), dir, scan, opts, profiles)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.write != "" {
		return finishReferenceWrite(prog, opts, f, files, notes, stdout, stderr)
	}
	return printComposeRef(stdout, stderr, stack, files, notes, f.jsonOut)
}

func finishReferenceWrite(prog string, o platformimport.EmitOptions, f composeRefFlags, files []platformimport.EmitFile, notes []string, stdout, stderr io.Writer) int {
	names, err := writeEmitted(f.write, files, f.force)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	next := []string{
		"turn on the runtime setting iac.compose_refs (beta, off by default) in Settings or through the settings API",
		fmt.Sprintf("%s apply -f %s --dry-run", prog, f.write),
		fmt.Sprintf("%s apply -f %s --yes", prog, f.write),
		fmt.Sprintf("%s stack adopt-from-export %s -f %s   (when the project already runs on Levelrail)", prog, o.Stack, f.write),
	}
	if f.jsonOut {
		if err := writeJSONValue(stdout, composeRefWriteResult{Dir: f.write, Stack: o.Stack, Files: names, Notes: notes, Next: next}); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "Wrote %d files to %s for stack %s (compose files untouched):\n", len(names), f.write, o.Stack)
	for _, n := range names {
		_, _ = fmt.Fprintf(stdout, "  %s\n", n)
	}
	for _, n := range notes {
		_, _ = fmt.Fprintf(stdout, "NOTE %s\n", n)
	}
	_, _ = fmt.Fprintf(stdout, "\nNEXT STEPS\n")
	for _, s := range next {
		_, _ = fmt.Fprintf(stdout, "  %s\n", s)
	}
	return exitOK
}

type composeRefWriteResult struct {
	Dir   string   `json:"dir"`
	Stack string   `json:"stack"`
	Files []string `json:"files"`
	Notes []string `json:"notes"`
	Next  []string `json:"next"`
}
