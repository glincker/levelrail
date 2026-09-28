package main

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/internal/store"
)

type oneBucket struct{ b cpbackup.Bucket }

func (o oneBucket) Open(context.Context, string) (cpbackup.Bucket, cpbackup.Location, error) {
	return o.b, cpbackup.Location{TargetID: "t"}, nil
}

type drFixture struct {
	srv      *objectstoretest.Server
	identity string
	key      string
	dataDir  string
}

func newDRFixture(t *testing.T) drFixture {
	t.Helper()
	ctx := context.Background()
	src, err := store.Open(ctx, filepath.Join(t.TempDir(), storeFilename))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = src.Close() })
	srv := objectstoretest.New("bk")
	t.Cleanup(srv.Close)
	client, err := objectstore.New(objectstore.Config{Endpoint: srv.URL, Bucket: "bk", AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true, HTTPClient: &http.Client{Timeout: 10 * time.Second}, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, err := cpbackup.GenerateIdentity(false)
	if err != nil {
		t.Fatal(err)
	}
	svc := cpbackup.NewService(src, src, oneBucket{client}, t.TempDir(), "v-test", cpbackup.DefaultOptions(), slog.New(slog.DiscardHandler))
	if err := svc.UpdateConfig(ctx, cpbackup.ConfigUpdate{Enabled: true, TargetID: "t", Recipients: []string{id.Recipient}}); err != nil {
		t.Fatal(err)
	}
	m, err := svc.RunBackup(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	idFile := filepath.Join(t.TempDir(), "identity.txt")
	if err := os.WriteFile(idFile, []byte(id.Secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return drFixture{srv: srv, identity: idFile, key: m.Key, dataDir: t.TempDir()}
}

func s3Env(k string) (string, bool) {
	switch k {
	case "AWS_ACCESS_KEY_ID":
		return "k", true
	case "AWS_SECRET_ACCESS_KEY":
		return "s", true
	}
	return "", false
}

func TestRunRestore_FromS3PrefixAndKey(t *testing.T) {
	f := newDRFixture(t)
	for _, from := range []string{"s3://bk/cp-backups/", "s3://bk/" + f.key} {
		dataDir := t.TempDir()
		var out bytes.Buffer
		err := runRestore(context.Background(), []string{"--from", from, "--identity", f.identity, "--endpoint", f.srv.URL}, dataDir, &out, s3Env)
		if err != nil {
			t.Fatalf("restore from %s: %v\n%s", from, err, out.String())
		}
		if _, err := store.InspectSnapshot(context.Background(), filepath.Join(dataDir, storeFilename)); err != nil {
			t.Fatalf("restored database invalid: %v", err)
		}
		if !strings.Contains(out.String(), "start the control plane") {
			t.Errorf("missing next steps: %q", out.String())
		}
	}
}

func TestRunRestore_FromFileAndDryRun(t *testing.T) {
	f := newDRFixture(t)
	dir := t.TempDir()
	data, _ := f.srv.Data(f.key)
	man, _ := f.srv.Data(cpbackup.ManifestKey(f.key))
	file := filepath.Join(dir, "backup.db.age")
	for path, b := range map[string][]byte{file: data, cpbackup.ManifestKey(file): man} {
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	live := filepath.Join(f.dataDir, storeFilename)
	if err := os.WriteFile(live, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runRestore(context.Background(), []string{"--from", file, "--identity", f.identity, "--dry-run"}, f.dataDir, &out, s3Env); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if got, _ := os.ReadFile(live); string(got) != "keep me" { //nolint:gosec // test path
		t.Fatal("dry run touched the live database")
	}
	if !strings.Contains(out.String(), "dry run complete") {
		t.Errorf("output = %q", out.String())
	}

	out.Reset()
	if err := runRestore(context.Background(), []string{"--from", file, "--identity", f.identity}, f.dataDir, &out, s3Env); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got, _ := os.ReadFile(live + ".pre-restore"); string(got) != "keep me" { //nolint:gosec // test path
		t.Fatalf("pre-restore copy = %q", got)
	}
}

func TestRunRestore_Refusals(t *testing.T) {
	f := newDRFixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		args []string
	}{
		{"missing flags", []string{"--from", "x"}},
		{"bad identity file", []string{"--from", "s3://bk/" + f.key, "--identity", filepath.Join(t.TempDir(), "nope"), "--endpoint", f.srv.URL}},
		{"unknown key", []string{"--from", "s3://bk/cp-backups/none/1.db.age", "--identity", f.identity, "--endpoint", f.srv.URL}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRestore(ctx, tc.args, f.dataDir, &bytes.Buffer{}, s3Env); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if err := runRestore(ctx, []string{"--from", "s3://bk/" + f.key, "--identity", f.identity, "--endpoint", f.srv.URL}, f.dataDir, &bytes.Buffer{}, func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("restore without credentials must fail")
	}
}

func TestRunRestore_RefusesRunningDatabase(t *testing.T) {
	f := newDRFixture(t)
	live := filepath.Join(f.dataDir, storeFilename)
	db, err := store.Open(context.Background(), live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	holder, err := sql.Open("sqlite", "file:"+live)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	conn, err := holder.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	err = runRestore(context.Background(), []string{"--from", "s3://bk/" + f.key, "--identity", f.identity, "--endpoint", f.srv.URL}, f.dataDir, &bytes.Buffer{}, s3Env)
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("err = %v, want an in-use refusal", err)
	}
}

func TestEscrowMaterialReader(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "master.key")
	if err := os.WriteFile(file, []byte("AGE-SECRET-KEY-PQ-1FAKE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, agentCAKeyFilename), []byte("ca key pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	none := func(string) string { return "" }
	got, err := escrowMaterialReader(file, dir, none)()
	if err != nil || got.MasterKey != "AGE-SECRET-KEY-PQ-1FAKE" || got.Files[agentCAKeyFilename] != "ca key pem" {
		t.Fatalf("file source = %+v, %v", got, err)
	}
	if _, ok := got.Files[agentCACertFilename]; ok {
		t.Fatal("a missing CA certificate must simply be left out")
	}
	got, err = escrowMaterialReader("", t.TempDir(), func(string) string { return " from-env " })()
	if err != nil || got.MasterKey != "from-env" || len(got.Files) != 0 {
		t.Fatalf("env source = %+v, %v", got, err)
	}
	if _, err := escrowMaterialReader("", dir, none)(); err == nil {
		t.Fatal("no source must error")
	}
}
