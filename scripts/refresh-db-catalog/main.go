// Command refresh-db-catalog builds the published database version catalog
// (databases.v1.json) and its ed25519 signature. See docs/database-version-catalog.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

const (
	catalogName = "databases.v1.json"
	runTimeout  = 10 * time.Minute
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}

func run(args []string, stdout, stderr io.Writer, lookup func(string) (string, bool)) int {
	fs := flag.NewFlagSet("refresh-db-catalog", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "catalog", "output directory")
	prevPath := fs.String("prev", "", "previous catalog for the shrink check (default: <out>/"+catalogName+")")
	keyEnv := fs.String("sign-key-env", "", "name of the env var holding the base64 ed25519 private key")
	genKey := fs.Bool("gen-key", false, "print a new key pair and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *genKey {
		priv, pub, err := dbcatalogbuild.GenerateKey()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "private (store as DB_CATALOG_SIGNING_KEY, never commit): %s\npublic (paste into the catalog public key constant): %s\n", priv, pub)
		return 0
	}
	var key string
	if *keyEnv != "" {
		key, _ = lookup(*keyEnv)
		if key == "" {
			_, _ = fmt.Fprintf(stderr, "refresh-db-catalog: env %s is empty\n", *keyEnv)
			return 1
		}
	}
	if err := publish(*out, *prevPath, key); err != nil {
		_, _ = fmt.Fprintln(stderr, "refresh-db-catalog:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, "wrote", filepath.Join(*out, catalogName))
	return 0
}

func publish(outDir, prevPath, signKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	target := filepath.Join(outDir, catalogName)
	if prevPath == "" {
		prevPath = target
	}
	var prev *dbcatalogbuild.File
	if raw, err := os.ReadFile(prevPath); err == nil { //nolint:gosec // operator-supplied CLI path
		p, perr := dbcatalogbuild.Parse(raw)
		if perr != nil {
			return fmt.Errorf("previous catalog: %w", perr)
		}
		prev = &p
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read previous catalog: %w", err)
	}
	b := &dbcatalogbuild.Builder{
		Tags:   dbcatalogbuild.RegistryTags{},
		Cycles: dbcatalogbuild.EOLClient{},
		Accept: func(engine, version string) bool { return database.ValidateEngineVersion(engine, version) == nil },
	}
	f, err := b.Build(ctx, dbcatalogbuild.DefaultSpecs())
	if err != nil {
		return err
	}
	if err := dbcatalogbuild.ValidateAgainst(f, prev); err != nil {
		return err
	}
	raw, err := f.Marshal()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	if signKey == "" {
		return nil
	}
	sig, err := dbcatalogbuild.Sign(signKey, raw)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target+".sig", []byte(sig+"\n"), 0o600); err != nil {
		return fmt.Errorf("write signature: %w", err)
	}
	return nil
}
