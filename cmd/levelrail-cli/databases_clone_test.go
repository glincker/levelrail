package main

import (
	"strings"
	"testing"
)

func TestDatabasesClone_PassesBackupID(t *testing.T) {
	var n int32
	var body string
	srv := exportFake(t, "succeeded", &n, &body)
	code, stdout, stderr := runDBCLI("databases", "clone", "mydb", "copy1", "--backup", "bkh_old", "--api-url", srv.URL)
	if code != exitOK || n != 0 || !strings.Contains(body, `"backup_id":"bkh_old"`) || !strings.Contains(body, `"new_name":"copy1"`) {
		t.Fatalf("code=%d triggers=%d body=%q stdout=%q stderr=%q", code, n, body, stdout, stderr)
	}
}

func TestDatabasesClone_TakesFreshBackupWithoutFlag(t *testing.T) {
	fastPoll(t)
	var n int32
	var body string
	srv := exportFake(t, "succeeded", &n, &body)
	code, _, stderr := runDBCLI("databases", "clone", "mydb", "copy1", "--api-url", srv.URL)
	if code != exitOK || n != 1 || !strings.Contains(body, `"backup_id":"bkh_new"`) {
		t.Fatalf("code=%d triggers=%d body=%q stderr=%q", code, n, body, stderr)
	}
}
