package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// dbImportFake answers the emptiness check and records every request.
func dbImportFake(t *testing.T, empty bool, posted *[]byte, calls *[]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		*calls = append(*calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/mydb/import":
			if empty {
				_, _ = io.WriteString(w, `{"database":"mydb","engine":"postgres","empty":true}`)
				return
			}
			_, _ = io.WriteString(w, `{"database":"mydb","engine":"postgres","empty":false}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/mydb/import":
			*posted, _ = io.ReadAll(r.Body)
			_, _ = io.WriteString(w, `{"database":"mydb","engine":"postgres","bytes":4,"replaced":false}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runDBImport(stdin io.Reader, args ...string) (int, string) {
	var o, e bytes.Buffer
	code := runDatabasesImport("levelrail-cli-test", args, stdin, &o, &e, envMap())
	return code, e.String()
}

func TestDatabasesImport_NonEmptyWithoutForceSendsNothing(t *testing.T) {
	var posted []byte
	var calls []string
	srv := dbImportFake(t, false, &posted, &calls)

	code, stderr := runDBImport(strings.NewReader("x"), "mydb", "--in", "-", "--api-url", srv.URL)
	if code == exitOK || !strings.Contains(stderr, "--force") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "POST") {
			t.Fatalf("an import request was sent: %v", calls)
		}
	}
}

func TestDatabasesImport_StdinForwardedIntact(t *testing.T) {
	var posted []byte
	var calls []string
	srv := dbImportFake(t, true, &posted, &calls)
	dump := []byte("PGDMP\x00\x01\xff\xfe line\r\n\x00tail")

	code, stderr := runDBImport(bytes.NewReader(dump), "mydb", "--in", "-", "--api-url", srv.URL)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !bytes.Equal(posted, dump) {
		t.Errorf("posted %q, want %q", posted, dump)
	}
}
