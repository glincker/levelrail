package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
)

const defaultIdentityFile = "levelrail-backup-identity.txt"

// runKeysGenerate makes an age keypair locally: the private key is written to
// a 0600 file on this machine and never sent anywhere.
func runKeysGenerate(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" control-plane-backups keys generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", defaultIdentityFile, "file to write the private identity to (mode 0600, never overwritten)")
	hybrid := fs.Bool("hybrid", false, "make a post-quantum hybrid key (needs age 1.3 or newer to decrypt outside this tool)")
	asJSON := fs.Bool("json", false, "print {\"recipient\", \"identity_file\"} as JSON to stdout and nothing else")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	secret, recipient, err := newAgeIdentity(*hybrid)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitValidation
	}
	body := fmt.Sprintf("# created: %s\n# public key: %s\n%s\n", time.Now().UTC().Format(time.RFC3339), recipient, secret)
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // operator-chosen output path
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "cannot create %s (it must not already exist): %v\n", *out, err)
		return exitValidation
	}
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		_, _ = fmt.Fprintf(stderr, "write %s: %v\n", *out, err)
		return exitValidation
	}
	if err := f.Close(); err != nil {
		_, _ = fmt.Fprintf(stderr, "write %s: %v\n", *out, err)
		return exitValidation
	}
	_, _ = fmt.Fprintf(stderr, "\nWARNING: %s is the only way to decrypt your control plane backups.\n"+
		"Store it OFFLINE (password manager, hardware token, printed copy in a safe), on a machine that is not\n"+
		"this server, and keep a second copy elsewhere. If it is lost the backups are unrecoverable. If it is\n"+
		"stolen along with a backup, your database is readable. Do not commit it or upload it next to the backups.\n\n", *out)
	if *asJSON {
		if err := json.NewEncoder(stdout).Encode(map[string]string{"recipient": recipient, "identity_file": *out}); err != nil {
			return exitValidation
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "%s\n", recipient)
	return exitOK
}

func newAgeIdentity(hybrid bool) (secret, recipient string, err error) {
	if hybrid {
		id, err := age.GenerateHybridIdentity()
		if err != nil {
			return "", "", fmt.Errorf("generate identity: %w", err)
		}
		return id.String(), id.Recipient().String(), nil
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("generate identity: %w", err)
	}
	return id.String(), id.Recipient().String(), nil
}

func runDREscrow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && args[0] == "open" {
		return runEscrowOpen(prog, args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "ack" {
		return runEscrowAck(prog, args[1:], stdout, stderr, lookupEnv)
	}
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups escrow", "print the escrow summary as JSON to stdout and nothing else", stderr)
	out := fs.String("out", "", "escrow file to write (default levelrail-escrow-<time>.age, mode 0600)")
	upload := fs.Bool("upload", false, "also upload the bundle to the separate escrow destination (opt-in; never the backup bucket)")
	ack := fs.Bool("ack", false, "after writing the file, record that you stored it offline")
	var recipients stringList
	fs.Var(&recipients, "recipient", "age public key to encrypt the escrow to (repeatable; default: the configured backup recipients)")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	ctx := context.Background()
	bundle, err := client.BuildControlPlaneEscrow(ctx, recipients, *upload)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("build escrow bundle: %w", err))
	}
	path := *out
	if path == "" {
		path = "levelrail-escrow-" + time.Now().UTC().Format("20060102T150405Z") + ".age"
	}
	if err := writeExclusive(path, bundle.Armored, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := writeExclusive(path+".instructions.txt", bundle.Instructions+"\n", 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	_, _ = fmt.Fprintf(stderr, "\nWARNING: %s holds your master key and agent CA key, encrypted to %d recipient(s).\n"+
		"Store it OFFLINE and NOT in the same place as your backups: whoever gets both a backup and this file\n"+
		"(plus a recipient's private key) can read every secret. Regenerate it after rotating the master key.\n\n", path, bundle.RecipientCount)
	if *ack {
		if _, err := client.AckControlPlaneEscrow(ctx); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("acknowledge escrow: %w", err))
		}
	}
	summary := map[string]any{"file": path, "instructions": path + ".instructions.txt", "fingerprint": bundle.Fingerprint, "recipients": bundle.RecipientCount, "uploaded_key": bundle.UploadedKey, "acknowledged": *ack}
	if err := renderResult(stdout, of.Format, of.Query, summary, func() {
		_, _ = fmt.Fprintf(stdout, "escrow bundle written to %s (fingerprint %s)\n", path, bundle.Fingerprint)
		if bundle.UploadedKey != "" {
			_, _ = fmt.Fprintf(stdout, "also uploaded to the escrow destination as %s\n", bundle.UploadedKey)
		}
		if !*ack {
			_, _ = fmt.Fprintf(stdout, "once it is stored offline, run: %s control-plane-backups escrow ack (or confirm it in Settings)\n", prog)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runEscrowAck(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups escrow ack", "print the new status as JSON to stdout and nothing else", stderr)
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	st, err := client.AckControlPlaneEscrow(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("acknowledge escrow: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, st, func() {
		_, _ = fmt.Fprintln(stdout, "escrow recorded as stored offline")
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func writeExclusive(path, content string, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) //nolint:gosec // operator-chosen output path
	if err != nil {
		return fmt.Errorf("cannot create %s (it must not already exist): %w", path, err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// runEscrowOpen decrypts an escrow bundle on this machine. It prints the master
// key, or with --extract writes master.key and the other recovery files into a
// directory.
func runEscrowOpen(prog string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(prog+" control-plane-backups escrow open", flag.ContinueOnError)
	fs.SetOutput(stderr)
	identity := fs.String("identity", "", "age identity file that matches one of the bundle's recipients")
	extract := fs.String("extract", "", "write master.key and the other recovery files (agent CA) into this directory instead of printing the key")
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() != 1 || *identity == "" {
		_, _ = fmt.Fprintf(stderr, "usage: %s control-plane-backups escrow open <bundle> --identity <file> [--extract DIR]\n", prog)
		return exitUsage
	}
	payload, err := openEscrowBundle(fs.Arg(0), *identity)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitValidation
	}
	if *extract == "" {
		_, _ = fmt.Fprintln(stderr, "The master key follows on stdout. Put it in master.key (mode 0600) or APP_MASTER_KEY and do not save it anywhere else.")
		_, _ = fmt.Fprintln(stdout, payload.MasterKey)
		return exitOK
	}
	written, err := extractEscrow(payload, *extract)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitValidation
	}
	for _, name := range written {
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(*extract, name))
	}
	_, _ = fmt.Fprintln(stderr, "Copy these files into the new server's data directory before starting it. Delete this directory once you have.")
	return exitOK
}

type escrowPayload struct {
	MasterKey string            `json:"master_key"`
	Files     map[string]string `json:"files"`
}

func extractEscrow(p escrowPayload, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	files := map[string]string{"master.key": p.MasterKey + "\n"}
	for name, content := range p.Files {
		if name != filepath.Base(name) || name == "." || name == ".." || name == "master.key" {
			return nil, fmt.Errorf("refusing to write a file named %q", name)
		}
		files[name] = content
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeExclusive(filepath.Join(dir, name), files[name], 0o600); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func openEscrowBundle(bundlePath, identityPath string) (escrowPayload, error) {
	raw, err := os.ReadFile(bundlePath) //nolint:gosec // operator-supplied path
	if err != nil {
		return escrowPayload{}, fmt.Errorf("read bundle: %w", err)
	}
	idf, err := os.Open(identityPath) //nolint:gosec // operator-supplied path
	if err != nil {
		return escrowPayload{}, fmt.Errorf("open identity file: %w", err)
	}
	defer func() { _ = idf.Close() }()
	ids, err := age.ParseIdentities(idf)
	if err != nil {
		return escrowPayload{}, fmt.Errorf("parse identity file: %w", err)
	}
	i := bytes.Index(raw, []byte(armor.Header))
	if i < 0 {
		return escrowPayload{}, errors.New("not an escrow bundle")
	}
	dec, err := age.Decrypt(armor.NewReader(bytes.NewReader(raw[i:])), ids...)
	if err != nil {
		return escrowPayload{}, errors.New("this identity cannot decrypt the bundle")
	}
	data, err := io.ReadAll(io.LimitReader(dec, 1<<20))
	if err != nil {
		return escrowPayload{}, fmt.Errorf("decrypt bundle: %w", err)
	}
	var payload escrowPayload
	if err := json.Unmarshal(data, &payload); err != nil || payload.MasterKey == "" {
		return escrowPayload{}, errors.New("the bundle does not contain a master key")
	}
	return payload, nil
}
