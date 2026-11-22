package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// runControlPlaneMove is the guided version of the "control plane machine is
// gone" runbook: it takes a fresh off-box backup, writes the key bundle and
// prints the exact commands and DNS cutover checklist for the new host.
func runControlPlaneMove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, urlP, profileP, jsonP, outP, queryP := apiFlagSet(prog, "control-plane-backups move", "print the move plan as JSON to stdout and nothing else", stderr)
	host := fs.String("to-host", "", "address of the new server (required)")
	sshUser := fs.String("ssh-user", "root", "SSH user on the new server")
	dnsName := fs.String("dns-name", "", "DNS name that points at the control plane (for the cutover checklist)")
	dir := fs.String("out", ".", "directory for the key bundle")
	skipBackup := fs.Bool("skip-backup", false, "reuse the newest off-box backup instead of taking a new one")
	var recipients stringList
	fs.Var(&recipients, "recipient", "age public key for the key bundle (repeatable; default: the configured backup recipients)")
	client, jsonOut, of, code, ok := drClient(prog, fs, args, apiFlagPtrs{tokenP, urlP, profileP, jsonP, outP, queryP}, stderr, lookupEnv)
	if !ok {
		return code
	}
	if *host == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --to-host is required\n", prog)
		return exitUsage
	}
	ctx, cancel := context.WithTimeout(context.Background(), drWaitLimit)
	defer cancel()

	if !*skipBackup {
		if err := client.RunControlPlaneOffboxBackup(ctx); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("start backup: %w", err))
		}
		st, err := waitDR(ctx, client, false)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		if st.LastBackupError != "" {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("backup failed: %s", st.LastBackupError))
		}
	}
	bundle, err := client.BuildControlPlaneEscrow(ctx, recipients, false)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("build key bundle: %w", err))
	}
	bundlePath := filepath.Join(*dir, "levelrail-escrow-"+time.Now().UTC().Format("20060102T150405Z")+".age")
	if err := writeExclusive(bundlePath, bundle.Armored, 0o600); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	plan, err := client.GetControlPlaneRestorePlan(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get restore plan: %w", err))
	}
	if plan.Backup == nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("no complete off-box backup at the destination"))
	}

	target := *sshUser + "@" + *host
	steps := moveSteps(prog, target, bundlePath, plan.Command, *dnsName)
	result := map[string]any{"bundle": bundlePath, "backup": plan.Backup.Key, "target": target, "steps": steps, "warnings": plan.Warnings}
	if err := renderResult(stdout, of.Format, of.Query, result, func() {
		_, _ = fmt.Fprintf(stdout, "Backup: %s (%s)\nKey bundle: %s (a recipient's private key opens it)\n\n", plan.Backup.Key, plan.Backup.CreatedAt, bundlePath)
		for i, s := range steps {
			_, _ = fmt.Fprintf(stdout, "%d. %s\n", i+1, s)
		}
		for _, w := range plan.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	_, _ = fmt.Fprintf(stderr, "\nStore %s offline, away from your backups. Delete the copy on the new host once restored.\n", bundlePath)
	return exitOK
}

func moveSteps(prog, target, bundle, restoreCmd, dnsName string) []string {
	dns := "the DNS name of the control plane"
	if dnsName != "" {
		dns = dnsName
	}
	if restoreCmd == "" {
		restoreCmd = "levelrail restore --from s3://<bucket>/cp-backups/<install-id>/ --identity backup-identity.txt"
	}
	return []string{
		fmt.Sprintf("Lower the TTL of %s to 60 seconds now, and wait for the old TTL to expire.", dns),
		fmt.Sprintf("Install the same Levelrail release and Docker on %s. Do not start it yet.", target),
		fmt.Sprintf("Ship the key bundle: scp %s %s:/root/escrow.age", bundle, target),
		fmt.Sprintf("On the new host recover the keys: %s control-plane-backups escrow open /root/escrow.age --identity backup-identity.txt --extract /var/lib/levelrail-data", prog),
		fmt.Sprintf("Rehearse on the new host with the restore command plus --dry-run: %s", restoreCmd),
		"Stop the old control plane so it stops reconciling. Agents keep running their containers.",
		fmt.Sprintf("Restore on the new host (APP_DATA_DIR=/var/lib/levelrail-data): %s", restoreCmd),
		"Start the control plane on the new host with the same APP_* settings as before.",
		fmt.Sprintf("Cutover: point %s at the new host, then raise the TTL again.", dns),
		fmt.Sprintf("Verify: %s apps list, %s nodes list (agents reconnect), then %s control-plane-backups run-now.", prog, prog, prog),
		"Keep the old host stopped for a few days, then decommission it.",
	}
}
