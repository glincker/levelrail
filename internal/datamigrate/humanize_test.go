package datamigrate

import (
	"strings"
	"testing"
)

func TestHumanize(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		code    string
		contain string
	}{
		{"timeout", `dump of the source failed: docker: exec [sh -c export PGHOST="$SRC_HOST" exec pg_dump --no-password] in a5739: exited 1: pg_dump: error: connection to server at "72.61.8.70", port 5432 failed: timeout expired`, FailureTimeout, "72.61.8.70:5432"},
		{"auth", `pg_dump: error: connection to server at "db.example.com", port 5432 failed: FATAL:  password authentication failed for user "app"`, FailureAuth, "db.example.com:5432"},
		{"mysql auth", `ERROR 1045 (28000): Access denied for user 'root'@'10.0.0.2'`, FailureAuth, "the source"},
		{"missing db", `FATAL:  database "geokit" does not exist`, FailureNoDatabase, `"geokit"`},
		{"ssl", `FATAL:  no pg_hba.conf entry for host "1.2.3.4", user "u", database "d", no encryption`, FailureSSL, "encrypted"},
		{"permission", `pg_dump: error: permission denied for table users`, FailurePermission, "not allowed"},
		{"disk", `pg_restore: error: could not write: No space left on device`, FailureDisk, "disk space"},
		{"refused", `psql: error: connection to server at "10.1.1.1", port 5432 failed: Connection refused`, FailureRefused, "10.1.1.1:5432"},
		{"dns", `could not translate host name "nope.invalid" to address`, FailureDNS, "resolved"},
		{"not running", `the managed database is not running yet, wait for it to become ready and retry`, FailureNotRunning, "not running"},
		{"mismatch", `1 of 2 tables differ: public.users source=10 target=9`, FailureMismatch, "do not match"},
		{"unknown", `something odd`, FailureUnknown, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Humanize(tt.raw, "", 0)
			if got.Code != tt.code {
				t.Fatalf("code = %q, want %q (%+v)", got.Code, tt.code, got)
			}
			if !strings.Contains(got.Summary, tt.contain) {
				t.Errorf("summary %q does not contain %q", got.Summary, tt.contain)
			}
			if got.Hint == "" || strings.Contains(got.Summary, "exec [sh") {
				t.Errorf("hint empty or summary leaks raw text: %+v", got)
			}
		})
	}
}

func TestHumanizeFallbackEndpoint(t *testing.T) {
	got := Humanize("timeout expired", "src.example.com", 3306)
	if !strings.Contains(got.Summary, "src.example.com:3306") {
		t.Errorf("summary = %q", got.Summary)
	}
}

func TestRedact(t *testing.T) {
	tests := []struct {
		name, in string
		gone     string
	}{
		{"env", `export SRC_PASSWORD=hunter2 exec pg_dump`, "hunter2"},
		{"quoted", `PGPASSWORD="s3cr et" pg_dump`, "s3cr"},
		{"uri", `postgres://app:topsecret@db.example.com:5432/app`, "topsecret"},
		{"mysql flag", `mysqldump -uroot -pmypass db`, "mypass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.in)
			if strings.Contains(got, tt.gone) {
				t.Errorf("Redact(%q) = %q still has the secret", tt.in, got)
			}
		})
	}
	if h := Humanize(`postgres://a:topsecret@h/db timeout expired`, "", 0); strings.Contains(h.Technical, "topsecret") {
		t.Errorf("technical leaks secret: %q", h.Technical)
	}
}
