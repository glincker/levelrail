package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintDataCopyFailureIsHumanisedUnlessVerbose(t *testing.T) {
	rows := []apiclient.DataCopyStatus{{
		Database: "geokit", Engine: "postgres", Status: "failed",
		Reason: "raw docker exec dump",
		Failure: &apiclient.DataCopyFailure{
			Code: "source_timeout", Summary: "The source database did not answer on 72.61.8.70:5432 in time.",
			Hint: "Check that it is reachable from this server.", Technical: "docker: exec [sh -c ...]: timeout expired",
		},
	}}
	tests := []struct {
		name    string
		verbose bool
		want    string
		notWant string
	}{
		{"default hides technical", false, "add --verbose", "docker: exec"},
		{"verbose shows technical", true, "docker: exec", "add --verbose"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printDataCopy(&buf, rows, true, tt.verbose)
			out := buf.String()
			if !strings.Contains(out, "did not answer on 72.61.8.70:5432") || !strings.Contains(out, tt.want) || strings.Contains(out, tt.notWant) || strings.Contains(out, "raw docker exec dump") {
				t.Errorf("output = %q", out)
			}
		})
	}
}
