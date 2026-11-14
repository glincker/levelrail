package main

import (
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// maxIssueLines caps the text listing so a broken tree stays readable; --json carries every issue.
const maxIssueLines = 20

// printIssueLines writes one "file:line path code message" row per issue, never any value.
func printIssueLines(w io.Writer, issues []iac.Issue) {
	for i, is := range issues {
		if i == maxIssueLines {
			_, _ = fmt.Fprintf(w, "... and %d more\n", len(issues)-maxIssueLines)
			return
		}
		code := is.Code
		if code == "" {
			code = iac.CodeInvalidDocument
		}
		_, _ = fmt.Fprintf(w, "  %s:%d %s %s %s\n", is.File, is.Line, is.Path, code, is.Message)
	}
}
