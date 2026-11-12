// Command gen-proven-templates rewrites internal/catalog/proven.json from a
// proof ledger CSV. A template is proven when its local_install column is
// "pass". Re-run it after more templates have been proven.
//
//	go run ./scripts/gen-proven-templates -ledger path/to/ledger.csv
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func main() {
	ledger := flag.String("ledger", "", "proof ledger CSV with id and local_install columns")
	out := flag.String("out", "internal/catalog/proven.json", "file to write")
	flag.Parse()
	if *ledger == "" {
		fmt.Fprintln(os.Stderr, "gen-proven-templates: -ledger is required")
		os.Exit(2)
	}
	f, err := os.Open(*ledger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-proven-templates:", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()
	ids, err := provenIDs(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-proven-templates:", err)
		os.Exit(1)
	}
	var skipped []string
	kept := ids[:0]
	for _, id := range ids {
		if _, ok := catalog.TemplateByID(id); ok {
			kept = append(kept, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	body, _ := json.MarshalIndent(kept, "", " ")
	if err := os.WriteFile(*out, append(body, '\n'), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "gen-proven-templates:", err)
		os.Exit(1)
	}
	fmt.Printf("proven %d, not in catalog (skipped) %d\n", len(kept), len(skipped))
}

// provenIDs returns the sorted ids whose local_install is "pass".
func provenIDs(r io.Reader) (ids []string, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("empty ledger")
	}
	idCol, passCol := -1, -1
	for i, h := range rows[0] {
		switch h {
		case "id":
			idCol = i
		case "local_install":
			passCol = i
		}
	}
	if idCol < 0 || passCol < 0 {
		return nil, errors.New("ledger needs id and local_install columns")
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if len(row) <= idCol || len(row) <= passCol || row[passCol] != "pass" || seen[row[idCol]] {
			continue
		}
		seen[row[idCol]] = true
		ids = append(ids, row[idCol])
	}
	sort.Strings(ids)
	return ids, nil
}
