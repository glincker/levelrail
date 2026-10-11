// Command import-templates converts a read-only clone of an upstream
// service template catalog into manifests under internal/catalog/manifests
// and writes a JSON report of what converted and why the rest was skipped.
// Without -write nothing on disk changes.
//
//	go run ./scripts/import-templates -src <compose dir> -select selected.txt -write
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/catalog/importer"
)

const (
	statusCommitted   = "committed"
	statusConvertible = "convertible"
	statusSkipped     = "skipped"
)

type reportEntry struct {
	ID      string            `json:"id"`
	Status  string            `json:"status"`
	Reason  string            `json:"reason,omitempty"`
	Detail  string            `json:"detail,omitempty"`
	Legacy  bool              `json:"already_in_legacy_catalog,omitempty"`
	Source  string            `json:"source"`
	Notes   []importer.Note   `json:"notes,omitempty"`
	Summary *importer.Summary `json:"summary,omitempty"`
}

type report struct {
	Source      string         `json:"source"`
	License     string         `json:"license"`
	Total       int            `json:"total"`
	Committed   int            `json:"committed"`
	Convertible int            `json:"convertible"`
	Skipped     int            `json:"skipped"`
	SkipReasons map[string]int `json:"skip_reasons"`
	Entries     []reportEntry  `json:"templates"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import-templates", flag.ContinueOnError)
	fs.SetOutput(stderr)
	src := fs.String("src", "", "directory of upstream compose template files (*.yaml)")
	srcURL := fs.String("source-url", "https://github.com/coollabsio/coolify", "upstream repository recorded in each manifest")
	license := fs.String("license", "Apache-2.0", "upstream licence recorded in each manifest")
	srcPrefix := fs.String("source-path", "templates/compose", "upstream path of -src, recorded in each manifest")
	out := fs.String("out", "internal/catalog/manifests", "manifests directory")
	selectFile := fs.String("select", "", "file with one template id per line; only these are written (default: none)")
	all := fs.Bool("all", false, "write every convertible template")
	write := fs.Bool("write", false, "write manifests and the report")
	reportPath := fs.String("report", "", "write the JSON report here")
	docsPath := fs.String("docs", "", "write the generated docs page here")
	list := fs.Bool("list", false, "print a table of every convertible template")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *src == "" {
		_, _ = fmt.Fprintln(stderr, "import-templates: -src is required")
		return 2
	}
	selected, err := readSelection(*selectFile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	legacy := legacyIDs()
	files, err := filepath.Glob(filepath.Join(*src, "*.y*ml"))
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	sort.Strings(files)

	rep := report{Source: *srcURL, License: *license, SkipReasons: map[string]int{}}
	for _, f := range files {
		id := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(f), ".yaml"), ".yml")
		body, err := os.ReadFile(f) //nolint:gosec // operator-supplied template directory, read only
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		res := importer.Convert(id, body, importer.Options{
			SourcePath: *srcPrefix + "/" + filepath.Base(f), SourceURL: *srcURL, License: *license,
		})
		e := reportEntry{ID: id, Source: res.Source, Notes: res.Notes, Summary: res.Summary, Legacy: legacy[id]}
		switch {
		case !res.Imported:
			e.Status, e.Reason, e.Detail = statusSkipped, res.Skip.Code, res.Skip.Detail
			rep.Skipped++
			rep.SkipReasons[res.Skip.Code]++
		case *all || selected[id]:
			e.Status = statusCommitted
			rep.Committed++
			if *write {
				if err := writeManifest(*out, res); err != nil {
					_, _ = fmt.Fprintln(stderr, err)
					return 1
				}
			}
		default:
			e.Status = statusConvertible
			rep.Convertible++
		}
		rep.Entries = append(rep.Entries, e)
	}
	rep.Total = len(rep.Entries)
	if *write {
		if err := pruneStale(*out, rep.Entries); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if *list {
		printList(stdout, rep)
	}
	_, _ = fmt.Fprintf(stdout, "templates %d, committed %d, convertible %d, skipped %d\n", rep.Total, rep.Committed, rep.Convertible, rep.Skipped)
	for _, code := range sortedKeys(rep.SkipReasons) {
		_, _ = fmt.Fprintf(stdout, "  skipped %-32s %d\n", code, rep.SkipReasons[code])
	}
	if *write && *docsPath != "" {
		if err := os.WriteFile(*docsPath, []byte(renderDocsPage(rep)), 0o600); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if *write && *reportPath != "" {
		if err := writeReport(*reportPath, rep); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func readSelection(path string) (map[string]bool, error) {
	out := map[string]bool{}
	if path == "" {
		return out, nil
	}
	f, err := os.Open(path) //nolint:gosec // operator-supplied selection file
	if err != nil {
		return nil, fmt.Errorf("import-templates: open selection: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			out[line] = true
		}
	}
	return out, sc.Err()
}

// legacyIDs lists catalog entries that predate the manifest format.
func legacyIDs() map[string]bool {
	out := map[string]bool{}
	for _, t := range catalog.Templates {
		if t.Manifest == nil {
			out[t.ID] = true
		}
	}
	return out
}

func writeManifest(root string, res importer.Output) error {
	dir := filepath.Join(root, res.ID)
	if existing, err := os.ReadFile(filepath.Join(dir, "template.yaml")); err == nil && !strings.Contains(string(existing), "\nsource:") { //nolint:gosec // path built from a validated template id
		return fmt.Errorf("import-templates: %s is a hand-written manifest, refusing to overwrite it", res.ID)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("import-templates: create %s: %w", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "template.yaml"), res.Template, 0o600); err != nil {
		return fmt.Errorf("import-templates: write template: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), res.Compose, 0o600); err != nil {
		return fmt.Errorf("import-templates: write compose: %w", err)
	}
	return nil
}

// pruneStale removes previously imported manifests (the ones carrying a
// source block) that this run no longer commits. Hand-written manifests
// have no source block and are left alone.
func pruneStale(root string, entries []reportEntry) error {
	keep := map[string]bool{}
	for _, e := range entries {
		if e.Status == statusCommitted {
			keep[e.ID] = true
		}
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("import-templates: read %s: %w", root, err)
	}
	for _, d := range dirs {
		if !d.IsDir() || keep[d.Name()] {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, d.Name(), "template.yaml")) //nolint:gosec // entry of the manifests directory
		if err != nil || !strings.Contains(string(body), "\nsource:") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, d.Name())); err != nil {
			return fmt.Errorf("import-templates: prune %s: %w", d.Name(), err)
		}
	}
	return nil
}

func writeReport(path string, rep report) error {
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("import-templates: encode report: %w", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("import-templates: write report: %w", err)
	}
	return nil
}

func printList(w io.Writer, rep report) {
	for _, e := range rep.Entries {
		if e.Status == statusSkipped || e.Summary == nil {
			continue
		}
		legacy := ""
		if e.Legacy {
			legacy = " legacy"
		}
		_, _ = fmt.Fprintf(w, "%-28s %-16s svc=%d vol=%d prompts=%d notes=%d%s\n",
			e.ID, e.Summary.Category, len(e.Summary.Services), len(e.Summary.Volumes), len(e.Summary.UserPrompt), len(e.Notes), legacy)
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
