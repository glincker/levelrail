package cleanup

import (
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"
)

// RunsCSVHeader is the column order of the CSV export.
var RunsCSVHeader = []string{"id", "target", "started_at", "finished_at", "days", "dry_run", "rows", "bytes", "trigger", "actor", "error"}

// WriteRunsCSV writes runs as CSV. Cells that a spreadsheet would read as a formula are prefixed with a quote.
func WriteRunsCSV(w io.Writer, runs []RunRecord) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(RunsCSVHeader); err != nil {
		return err
	}
	for _, r := range runs {
		row := []string{
			strconv.FormatInt(r.ID, 10), safeCell(r.Target), r.StartedAt.UTC().Format(time.RFC3339), r.FinishedAt.UTC().Format(time.RFC3339),
			strconv.Itoa(r.Days), strconv.FormatBool(r.DryRun), strconv.FormatInt(r.Rows, 10), strconv.FormatInt(r.Bytes, 10),
			safeCell(r.Trigger), safeCell(r.Actor), safeCell(r.Error),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
