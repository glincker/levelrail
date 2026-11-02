package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func portForwardUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s port-forward <app|db>/<name> [LOCAL:]REMOTE [--service S] [flags]

Opens a loopback-only local port that relays to an app container port or a
managed database through the control plane. A bare name means an app. For a
database the remote port is the database's own and may be left out.

  %[1]s port-forward web 8080:3000       app web, container port 3000
  %[1]s port-forward db/main 5433        database main on local port 5433

Flags:
  --service S     service of a multi-service app (default: the app itself)
  --json          print the listening address as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

// parseForwardTarget splits "db/main", "app/web" or "web" into a kind word and a name.
func parseForwardTarget(s string) (isDB bool, name string, err error) {
	kind, rest, found := strings.Cut(s, "/")
	if !found {
		return false, s, nil
	}
	k, kerr := resolveResourceKind(kind)
	if kerr != nil || k.Path == "nodes" || rest == "" {
		return false, "", newValidationError("target %q must be app/NAME, db/NAME or a bare app name", s)
	}
	return k.Path == "databases", rest, nil
}

// parsePortPair reads "LOCAL:REMOTE", "PORT" (same both sides) or "" (free local port).
func parsePortPair(s string) (local, remote int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	l, r, two := strings.Cut(s, ":")
	if local, err = parsePort(l, true); err != nil {
		return 0, 0, err
	}
	if !two {
		return local, local, nil
	}
	if remote, err = parsePort(r, false); err != nil {
		return 0, 0, err
	}
	return local, remote, nil
}

func parsePort(s string, allowEmpty bool) (int, error) {
	if s == "" && allowEmpty {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, newValidationError("invalid port %q, want 1 to 65535", s)
	}
	return n, nil
}

func runPortForward(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service *string
	c, code, ok := parseCLICall(prog, "port-forward", portForwardUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		service = fs.String("service", "", "service of a multi-service app")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return c.invalid("port-forward needs a target and LOCAL:REMOTE, for example: port-forward web 8080:3000")
	}
	isDB, name, err := parseForwardTarget(rest[0])
	if err != nil {
		return c.fail(err)
	}
	pair := ""
	if len(rest) == 2 {
		pair = rest[1]
	}
	local, remote, err := parsePortPair(pair)
	if err != nil {
		return c.fail(err)
	}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(local)))
	if err != nil {
		return c.fail(fmt.Errorf("listen on 127.0.0.1:%d: %w", local, err))
	}
	defer func() { _ = ln.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	kind := "app"
	dial := func(dctx context.Context) (net.Conn, error) {
		return c.client.DialAppTunnel(dctx, name, *service, remote)
	}
	if isDB {
		kind = "database"
		dial = func(dctx context.Context) (net.Conn, error) { return c.client.DialDatabaseTunnel(dctx, name) }
	}
	info := map[string]any{"kind": kind, "name": name, "listen": ln.Addr().String()}
	if c.jsonOut {
		_ = c.render(info, func() {})
	} else {
		_, _ = fmt.Fprintf(stdout, "forwarding %s %q on %s (Ctrl+C to stop)\n", kind, name, ln.Addr())
	}
	serveTunnel(ctx, ln, dial, stderr)
	return exitOK
}

func cpUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cp <app>/<volume>:<path> <local|->    download one file
  %[1]s cp <local|-> <app>/<volume>:<path>    upload one file
  %[1]s cp --from-backup ID <app>/<volume>:<path>   restore one file from a backup

A remote side is APP/VOLUME:PATH. Exactly one side is remote. Single files
unless -r is given, which copies a whole directory tree one file at a time.

Flags:
  -r, --recursive        copy a directory and everything under it
  --from-backup string   restore the remote path from this backup id instead of uploading
  --json                 print the transfer result as JSON and nothing else
%[2]s`, prog, commonFlagsHelp)
}

type remoteSpec struct{ App, Volume, Path string }

// parseRemoteSpec recognises APP/VOLUME:PATH; anything with a leading dot or slash stays a local path.
func parseRemoteSpec(s string) (remoteSpec, bool) {
	head, path, found := strings.Cut(s, ":")
	if !found || head == "" || strings.HasPrefix(head, ".") || strings.HasPrefix(head, "/") {
		return remoteSpec{}, false
	}
	app, vol, two := strings.Cut(head, "/")
	if !two || app == "" || vol == "" || strings.Contains(vol, "/") {
		return remoteSpec{}, false
	}
	return remoteSpec{App: app, Volume: vol, Path: path}, true
}

func runCp(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var backupID *string
	var recursive *bool
	c, code, ok := parseCLICall(prog, "cp", cpUsage(prog), expandShortFlags(args), stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		backupID = fs.String("from-backup", "", "restore the remote path from this backup id")
		recursive = fs.Bool("recursive", false, "copy a directory tree")
		fs.BoolVar(recursive, "r", false, "copy a directory tree (short)")
	})
	if !ok {
		return code
	}
	rest := c.fs.Args()
	ctx := context.Background()
	if *backupID != "" {
		r, isRemote := remoteArg(rest, 1)
		if !isRemote {
			return c.invalid("--from-backup needs one remote target like web/data:conf/app.ini")
		}
		res, err := c.client.RestoreVolumeFile(ctx, r.App, r.Volume, *backupID, r.Path)
		if err != nil {
			return c.fail(fmt.Errorf("restore %s from backup %s: %w", r.Path, *backupID, err))
		}
		return c.render(res, func() { _, _ = fmt.Fprintf(c.stdout, "restored %s (%d bytes)\n", res.Path, res.Size) })
	}
	if len(rest) != 2 {
		return c.invalid("cp needs a source and a destination, exactly one of them APP/VOLUME:PATH")
	}
	srcR, srcRemote := parseRemoteSpec(rest[0])
	dstR, dstRemote := parseRemoteSpec(rest[1])
	if srcRemote == dstRemote {
		return c.invalid("exactly one side must be remote, written APP/VOLUME:PATH")
	}
	if *recursive {
		if srcRemote {
			n, err := c.downloadTree(ctx, srcR, rest[1])
			if err != nil {
				return c.fail(fmt.Errorf("download %s: %w", rest[0], err))
			}
			return c.render(map[string]any{"path": srcR.Path, "files": n}, func() { _, _ = fmt.Fprintf(c.stdout, "downloaded %d files to %s\n", n, rest[1]) })
		}
		n, err := c.uploadTree(ctx, rest[0], dstR)
		if err != nil {
			return c.fail(fmt.Errorf("upload to %s: %w", rest[1], err))
		}
		return c.render(map[string]any{"path": dstR.Path, "files": n}, func() { _, _ = fmt.Fprintf(c.stdout, "uploaded %d files to %s\n", n, rest[1]) })
	}
	if srcRemote {
		w, closeFn, err := openLocalDest(rest[1], stdout)
		if err != nil {
			return c.fail(err)
		}
		n, err := c.client.DownloadVolumeFile(ctx, srcR.App, srcR.Volume, srcR.Path, w)
		if cerr := closeFn(err); err == nil {
			err = cerr
		}
		if err != nil {
			return c.fail(fmt.Errorf("download %s: %w", rest[0], err))
		}
		if rest[1] == "-" {
			return exitOK
		}
		return c.render(map[string]any{"path": srcR.Path, "size": n}, func() { _, _ = fmt.Fprintf(c.stdout, "downloaded %d bytes to %s\n", n, rest[1]) })
	}
	in := io.Reader(stdinSource)
	if rest[0] != "-" {
		f, err := os.Open(rest[0]) //nolint:gosec // the operator names the local file to upload
		if err != nil {
			return c.fail(fmt.Errorf("open %s: %w", rest[0], err))
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	res, err := c.client.UploadVolumeFile(ctx, dstR.App, dstR.Volume, dstR.Path, in)
	if err != nil {
		return c.fail(fmt.Errorf("upload to %s: %w", rest[1], err))
	}
	return c.render(res, func() { _, _ = fmt.Fprintf(c.stdout, "uploaded %d bytes to %s\n", res.Size, res.Path) })
}

func remoteArg(rest []string, want int) (remoteSpec, bool) {
	if len(rest) != want {
		return remoteSpec{}, false
	}
	return parseRemoteSpec(rest[want-1])
}

// uploadTree sends every regular file under root; symlinks are skipped so a link cannot pull in files outside the tree.
func (c cliCall) uploadTree(ctx context.Context, root string, dst remoteSpec) (int, error) {
	n := 0
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		f, oerr := os.Open(p) //nolint:gosec // the operator names the local tree to upload
		if oerr != nil {
			return oerr
		}
		defer func() { _ = f.Close() }()
		if _, uerr := c.client.UploadVolumeFile(ctx, dst.App, dst.Volume, path.Join(dst.Path, filepath.ToSlash(rel)), f); uerr != nil {
			return fmt.Errorf("%s: %w", rel, uerr)
		}
		n++
		return nil
	})
	return n, err
}

// downloadTree mirrors a remote directory into dest, listing one level at a time.
func (c cliCall) downloadTree(ctx context.Context, src remoteSpec, dest string) (int, error) {
	n := 0
	var walk func(remote, local string) error
	walk = func(remote, local string) error {
		listing, err := c.client.ListVolumeFiles(ctx, src.App, src.Volume, remote, 0)
		if err != nil {
			return err
		}
		if listing.Truncated {
			return fmt.Errorf("%s has more entries than one listing returns", remote)
		}
		if err := os.MkdirAll(local, 0o750); err != nil {
			return err
		}
		for _, e := range listing.Entries {
			// Base() keeps a hostile entry name from escaping the destination.
			name := filepath.Base(e.Name)
			if name == "." || name == ".." || name == string(filepath.Separator) {
				continue
			}
			rp, lp := path.Join(remote, name), filepath.Join(local, name)
			switch e.Type {
			case "dir":
				if err := walk(rp, lp); err != nil {
					return err
				}
			case "file":
				f, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // the path joins the operator's destination and a sanitised base name
				if err != nil {
					return err
				}
				_, derr := c.client.DownloadVolumeFile(ctx, src.App, src.Volume, rp, f)
				if cerr := f.Close(); derr == nil {
					derr = cerr
				}
				if derr != nil {
					return fmt.Errorf("%s: %w", rp, derr)
				}
				n++
			}
		}
		return nil
	}
	return n, walk(src.Path, dest)
}
