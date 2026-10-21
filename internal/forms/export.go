package forms

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// WriteCSV writes submissions with one column per field name seen across them.
// Cells a spreadsheet would run as a formula are prefixed with a quote.
func WriteCSV(w io.Writer, subs []store.FormSubmission) error {
	seen := map[string]struct{}{}
	for _, s := range subs {
		for k := range s.Fields {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	cw := csv.NewWriter(w)
	header := []string{"id", "created_at", "spam"}
	for _, c := range cols {
		header = append(header, csvCell(c))
	}
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("forms: write csv: %w", err)
	}
	for _, s := range subs {
		row := []string{fmt.Sprint(s.ID), s.CreatedAt.UTC().Format(time.RFC3339), fmt.Sprint(s.Spam)}
		for _, c := range cols {
			row = append(row, csvCell(s.Fields[c]))
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("forms: write csv: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func csvCell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}
