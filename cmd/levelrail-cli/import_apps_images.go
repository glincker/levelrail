package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envImportSSHPassphrase = "APP_IMPORT_SSH_PASSPHRASE" //nolint:gosec // env var name, not a credential
	maxSSHKeyBytes         = 64 << 10
	imageStateVerified     = "verified"
	imageStateLoaded       = "loaded"
)

type imageMoveFlags struct {
	transfer, list, cancel, agent bool
	ssh, key                      string
	port                          int
}

func (f *imageMoveFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.transfer, "transfer-images", false, "move host-built images from the source over SSH into this node (needs --session)")
	fs.BoolVar(&f.list, "images", false, "list the session's host-built images and their move state (needs --session)")
	fs.BoolVar(&f.cancel, "cancel-images", false, "cancel a running image move (needs --session)")
	fs.StringVar(&f.ssh, "ssh", "", "source SSH login for --transfer-images, as user@host or user@host:port")
	fs.IntVar(&f.port, "ssh-port", 0, "source SSH port (default 22)")
	fs.StringVar(&f.key, "ssh-key", "", "private key file for --ssh, or - to read it from stdin")
	fs.BoolVar(&f.agent, "ssh-agent", false, "sign with the control plane's own SSH agent instead of a key")
}

func (f imageMoveFlags) any() bool { return f.transfer || f.list || f.cancel }

func readSSHKey(path string, stdin io.Reader) (string, error) {
	if path == "" {
		return "", nil
	}
	r := stdin
	if path != "-" {
		fh, err := os.Open(path) //nolint:gosec // path is the operator's own CLI argument
		if err != nil {
			return "", fmt.Errorf("read ssh key: %w", err)
		}
		defer func() { _ = fh.Close() }()
		r = fh
	}
	b, err := io.ReadAll(io.LimitReader(r, maxSSHKeyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read ssh key: %w", err)
	}
	if len(b) > maxSSHKeyBytes {
		return "", errors.New("the ssh key file is larger than 64 KiB")
	}
	return string(b), nil
}

func runImportAppsImages(ctx context.Context, prog string, client *Client, f importAppsFlags, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	im := f.images
	switch {
	case im.cancel:
		v, err := client.CancelAppImportImages(ctx, f.session)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("cancel image move: %w", err))
		}
		return printAppImportImages(stdout, stderr, f.jsonOut, v, "Image move cancelled")
	case im.list:
		v, err := client.AppImportImages(ctx, f.session)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("list images: %w", err))
		}
		return printAppImportImages(stdout, stderr, f.jsonOut, v, "Host-built images")
	}
	key, err := readSSHKey(im.key, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	passphrase, _ := lookupEnv(envImportSSHPassphrase)
	body := apiclient.AppImportImagesTransfer{SSH: im.ssh, Port: im.port, PrivateKey: key, Passphrase: passphrase, UseAgent: im.agent, Items: f.onlyList()}
	v, err := client.TransferAppImportImages(ctx, f.session, body)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("start image move: %w", err))
	}
	deadline := time.Now().Add(f.wait)
	last := ""
	for v.Running && time.Now().Before(deadline) {
		if !f.jsonOut {
			if line := imageProgressLine(v); line != last {
				_, _ = fmt.Fprintln(stderr, line)
				last = line
			}
		}
		time.Sleep(appImportPollEvery)
		if v, err = client.AppImportImagesStatus(ctx, f.session); err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("poll image move: %w", err))
		}
	}
	code := printAppImportImages(stdout, stderr, f.jsonOut, v, "Image move")
	if !f.jsonOut && !v.Running {
		_, _ = fmt.Fprintf(stdout, "\nNext: %s import apps --session %s --verify\n", prog, f.session)
	}
	if v.Running {
		return exitCheckFailed
	}
	for _, img := range v.Images {
		if img.State != imageStateVerified && img.State != imageStateLoaded {
			return exitCheckFailed
		}
	}
	return code
}

func imageProgressLine(v apiclient.AppImportImages) string {
	var parts []string
	for _, img := range v.Images {
		parts = append(parts, fmt.Sprintf("%s %s %s", img.App, img.State, humanBytes(img.Bytes)))
	}
	return "moving: " + strings.Join(parts, ", ")
}

func printAppImportImages(stdout, stderr io.Writer, jsonOut bool, v apiclient.AppImportImages, title string) int {
	if jsonOut {
		if err := writeJSONValue(stdout, v); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	src := v.Source
	if src == "" {
		src = "no source login yet"
	}
	_, _ = fmt.Fprintf(stdout, "%s (%s)\n\n", title, src)
	if len(v.Images) == 0 {
		_, _ = fmt.Fprintln(stdout, "No selected app has an image built on the source host.")
		return exitOK
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tIMAGE\tSTATE\tBYTES\tVERIFIED")
	for _, img := range v.Images {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\n", img.App, img.Image, img.State, humanBytes(img.Bytes), img.Verified)
	}
	_ = tw.Flush()
	for _, img := range v.Images {
		if img.Error != "" {
			_, _ = fmt.Fprintf(stdout, "\n%s: %s\n", img.App, img.Error)
		}
	}
	if !v.Supported {
		_, _ = fmt.Fprintln(stdout, "\nThis control plane has no node runtime to load images into.")
	}
	return exitOK
}
