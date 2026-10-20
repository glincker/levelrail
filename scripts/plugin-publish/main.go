// Command plugin-publish signs plugin manifests and marketplace indexes offline.
// Subcommands: keygen, sign, entry, index-sign (see docs/plugin-publishing.md).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "plugin-publish:", err)
		os.Exit(1)
	}
}

const usage = "usage: plugin-publish keygen|sign|entry|index-sign [flags]"

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "keygen":
		return keygen(args[1:], out)
	case "sign":
		return signManifest(args[1:], out)
	case "entry":
		return buildEntry(args[1:], out)
	case "index-sign":
		return indexSign(args[1:], out)
	default:
		return errors.New(usage)
	}
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func loadKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the operator's own key file
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s is not a base64 ed25519 seed", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func keygen(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	path := fs.String("out", "", "key file to create (refuses to overwrite)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("keygen needs -out")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // the operator chooses the path
	if err != nil {
		return err
	}
	if _, err := f.WriteString(base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "public_key: %s\nkey_id:     %s\n", base64.StdEncoding.EncodeToString(pub), plugin.Fingerprint(pub))
	return err
}

func signManifest(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	manifest := fs.String("manifest", "plugin.yaml", "manifest to sign")
	keyPath := fs.String("key", "", "publisher key file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*manifest) //nolint:gosec // the publisher's own manifest
	if err != nil {
		return err
	}
	man, err := plugin.ParseManifest(raw)
	if err != nil {
		return err
	}
	priv, err := loadKey(*keyPath)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, plugin.Sign(priv, raw, man.Digest()))
	return err
}

func buildEntry(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("entry", flag.ContinueOnError)
	manifest := fs.String("manifest", "plugin.yaml", "signed manifest")
	sigPath := fs.String("signature", "", "file with the detached signature (from sign)")
	keyPath := fs.String("key", "", "publisher key file (only its public half is used)")
	manifestURL := fs.String("manifest-url", "", "https URL the manifest will be served from")
	name := fs.String("name", "", "display name (defaults to the plugin name)")
	summary := fs.String("summary", "", "one line summary (defaults to the manifest description)")
	icon := fs.String("icon", "", "BrandIcon key, empty for a monogram")
	changelog := fs.String("changelog", "", "what changed in this version")
	minVersion := fs.String("min-version", "", "oldest control plane version this works with")
	var categories listFlag
	fs.Var(&categories, "category", "category, repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*manifest) //nolint:gosec // the publisher's own manifest
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(*sigPath) //nolint:gosec // the publisher's own signature
	if err != nil {
		return err
	}
	priv, err := loadKey(*keyPath)
	if err != nil {
		return err
	}
	e, err := plugin.BuildEntry(raw, *manifestURL, strings.TrimSpace(string(sig)), priv.Public().(ed25519.PublicKey), plugin.Entry{
		Name: *name, Summary: *summary, Icon: *icon, Changelog: *changelog, MinVersion: *minVersion, Categories: categories,
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

func indexSign(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("index-sign", flag.ContinueOnError)
	keyPath := fs.String("key", "", "index root key file")
	outPath := fs.String("out", "", "file to write the signed index to (stdout when empty)")
	sequence := fs.Int64("sequence", 0, "monotonic sequence, must be higher than the last index you published")
	var entries, publishers listFlag
	fs.Var(&entries, "entry", "entry JSON file from the entry command, repeatable")
	fs.Var(&publishers, "publisher", "PUBLIC_KEY_B64=Name for each publisher key the index vouches for, repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	priv, err := loadKey(*keyPath)
	if err != nil {
		return err
	}
	idx := plugin.Index{Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Sequence: *sequence}
	for _, p := range publishers {
		b64, name, ok := strings.Cut(p, "=")
		pub, err := plugin.ParsePublicKey(b64)
		if !ok || err != nil {
			return fmt.Errorf("-publisher %q must be PUBLIC_KEY_B64=Name", p)
		}
		idx.PublisherKeys = append(idx.PublisherKeys, plugin.IndexKey{KeyID: plugin.Fingerprint(pub), Name: name, PublicKey: b64})
	}
	for _, path := range entries {
		raw, err := os.ReadFile(path) //nolint:gosec // the maintainer's own entry file
		if err != nil {
			return err
		}
		var e plugin.Entry
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		idx.Plugins = append(idx.Plugins, e)
	}
	signed, err := plugin.SignIndex(priv, idx)
	if err != nil {
		return err
	}
	if *outPath == "" {
		_, err = out.Write(append(signed, '\n'))
		return err
	}
	return os.WriteFile(*outPath, append(signed, '\n'), 0o600)
}
