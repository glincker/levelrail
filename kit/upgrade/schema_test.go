package upgrade

import (
	"context"
	"strings"
	"testing"
)

func TestSchemaCheck(t *testing.T) {
	tests := []struct {
		name       string
		in         SchemaInput
		wantStatus string
		wantFix    string
		wantMsg    string
	}{
		{"older than database", SchemaInput{Tag: "v0.6.0", State: SchemaKnown, DB: 2160, Release: 1010}, StatusFail, FixOpenRollback,
			"target release is older than the database schema: database is at schema 2160, v0.6.0 supports 1010"},
		{"equal", SchemaInput{Tag: "v1.0.0", State: SchemaKnown, DB: 10, Release: 10}, StatusOK, "", "same database schema"},
		{"newer migrates", SchemaInput{Tag: "v1.1.0", State: SchemaKnown, DB: 10, Release: 12}, StatusOK, "", "from schema 10 to 12"},
		{"database unreadable", SchemaInput{Tag: "v1.1.0", State: SchemaKnown, DB: -1, Release: 12}, StatusUnknown, "", "could not be read"},
		{"manifest missing", SchemaInput{Tag: "v0.5.0", State: SchemaMissing, DB: 10, Release: -1}, StatusUnknown, "", "checked after download"},
		{"manifest unavailable", SchemaInput{Tag: "v1.1.0", State: SchemaUnavailable, DB: 10, Release: -1}, StatusUnknown, FixRecheck, "checked after download"},
		{"manifest unverified", SchemaInput{Tag: "v1.1.0", State: SchemaUnverified, DB: 10, Release: -1, Detail: "bad signature"}, StatusWarn, FixRecheck, "bad signature"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := schemaCheck(tc.in)
			if c.Code != CodeSchemaCompat || c.Status != tc.wantStatus {
				t.Fatalf("code %q status %q, want %q", c.Code, c.Status, tc.wantStatus)
			}
			if !strings.Contains(c.Message, tc.wantMsg) {
				t.Fatalf("message %q lacks %q", c.Message, tc.wantMsg)
			}
			got := ""
			if c.Fix != nil {
				got = c.Fix.Action
			}
			if got != tc.wantFix {
				t.Fatalf("fix %q, want %q", got, tc.wantFix)
			}
			if tc.wantStatus != StatusFail && SchemaBlocked([]Check{c}) {
				t.Fatal("a non-failing schema check blocked")
			}
		})
	}
}

func TestRunIncludesSchemaCheckOnlyWhenGiven(t *testing.T) {
	in := baseInputs()
	if byCode(Run(context.Background(), in), CodeSchemaCompat).Code != "" {
		t.Fatal("schema check present without input")
	}
	in.Schema = &SchemaInput{Tag: "v0.6.0", State: SchemaKnown, DB: 20, Release: 10}
	checks := Run(context.Background(), in)
	if !SchemaBlocked(checks) || !Blocked(checks) {
		t.Fatalf("older release not blocked: %+v", byCode(checks, CodeSchemaCompat))
	}
	in.ReleaseKind = KindEdgeImage
	if byCode(Run(context.Background(), in), CodeSchemaCompat).Code != "" {
		t.Fatal("edge image got a schema check")
	}
}

func TestUpdateAvailableForDescribeBuild(t *testing.T) {
	if UpdateAvailable("v0.6.0-138-gabbfcdba5", &Release{Tag: "v0.6.0"}) {
		t.Fatal("a build 138 commits after v0.6.0 was offered v0.6.0")
	}
	if !UpdateAvailable("v0.6.0-138-gabbfcdba5", &Release{Tag: "v0.7.0"}) {
		t.Fatal("a newer release was not offered to a describe build")
	}
}
