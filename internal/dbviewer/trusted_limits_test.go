package dbviewer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
)

func largeSchemaCSV(t *testing.T, relations int) (csvText string, doc string) {
	t.Helper()
	type rel struct {
		Schema string   `json:"schema"`
		Name   string   `json:"name"`
		Cols   []string `json:"cols"`
	}
	rows := make([]rel, 0, relations)
	for i := 0; i < relations; i++ {
		rows = append(rows, rel{Schema: "public", Name: "table_" + strings.Repeat("x", 20), Cols: []string{"id", "created_at", "updated_at", "name", "payload"}})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"j"})
	_ = w.Write([]string{string(b)})
	w.Flush()
	return buf.String(), string(b)
}

func TestTrustedLimitsKeepLargeSchemaDocumentIntact(t *testing.T) {
	text, doc := largeSchemaCSV(t, 2000)
	if len(doc) <= defaultMaxCell {
		t.Fatalf("fixture must exceed the default cell cap: %d <= %d", len(doc), defaultMaxCell)
	}
	base := LimitsFromEnv(func(string) (string, bool) { return "", false })

	cases := []struct {
		name      string
		limits    Limits
		wantFull  bool
		wantValid bool
	}{
		{"user query keeps the cell cap", base, false, false},
		{"trusted introspection lifts it", trustedLimits(base), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := parseCSV(strings.NewReader(text), "\x00NULL", tc.limits)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Rows) != 1 || res.Rows[0][0] == nil {
				t.Fatalf("rows = %v", res.Rows)
			}
			got := *res.Rows[0][0]
			if full := got == doc; full != tc.wantFull {
				t.Fatalf("full document = %v, want %v (len %d of %d)", full, tc.wantFull, len(got), len(doc))
			}
			if valid := json.Valid([]byte(got)); valid != tc.wantValid {
				t.Fatalf("valid JSON = %v, want %v", valid, tc.wantValid)
			}
		})
	}
}

func TestTrustedLimitsNeverLowerAnExplicitCap(t *testing.T) {
	l := Limits{MaxBytes: 1 << 20, MaxCellBytes: 4 << 20}
	if got := trustedLimits(l); got.MaxCellBytes != 4<<20 {
		t.Fatalf("MaxCellBytes = %d, want unchanged", got.MaxCellBytes)
	}
}
