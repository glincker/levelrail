package main

import (
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/apiclient"
)

// printPagesDiagnosis prints the domain checklist, one "ok" or "fail" row per check.
func printPagesDiagnosis(out io.Writer, rows []apiclient.StaticSiteDiagnosis) {
	if len(rows) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "checks:")
	for _, r := range rows {
		mark := "fail"
		if r.OK {
			mark = "ok  "
		}
		_, _ = fmt.Fprintf(out, "  [%s] %s: %s\n", mark, r.Code, r.Hint)
	}
}
