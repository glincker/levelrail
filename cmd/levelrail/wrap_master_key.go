package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

// runWrapMasterKey seals the master key file with the configured external key service,
// or with --unwrap, writes the plain key back. The result is verified before it replaces the file.
func runWrapMasterKey(ctx context.Context, args []string, dataDir string, stdout io.Writer, getenv func(string) string) error {
	fs := flag.NewFlagSet("wrap-master-key", flag.ContinueOnError)
	unwrap := fs.Bool("unwrap", false, "write the plain master key back instead of wrapping it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	w, err := keyWrapperFromEnv(getenv)
	if err != nil {
		return err
	}
	if w == nil {
		return fmt.Errorf("set %s and its settings first, see docs/security.md", envKMSProvider)
	}
	path := filepath.Join(dataDir, masterKeyFilename)
	if custom := strings.TrimSpace(getenv(envMasterKeyFile)); custom != "" {
		path = custom
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled data directory path
	if err != nil {
		return fmt.Errorf("read master key file %s: %w (an env-sourced APP_MASTER_KEY is wrapped by hand: see docs/security.md)", path, err)
	}
	stored := strings.TrimSpace(string(raw))
	wrapped := secrets.IsWrappedMasterKey(stored)

	var out string
	switch {
	case *unwrap && !wrapped:
		return errors.New("the master key file is not wrapped")
	case *unwrap:
		if out, err = secrets.UnwrapMasterKey(ctx, stored, w); err != nil {
			return err
		}
	case wrapped:
		return errors.New("the master key file is already wrapped")
	default:
		if _, err := secrets.LoadMasterKey(stored); err != nil {
			return fmt.Errorf("the master key file does not hold a valid key: %w", err)
		}
		if out, err = secrets.WrapMasterKey(ctx, stored, w); err != nil {
			return err
		}
		back, err := secrets.UnwrapMasterKey(ctx, out, w)
		if err != nil || back != stored {
			return fmt.Errorf("refusing to replace the key file: the wrapped value did not round trip (%v)", err)
		}
	}
	if err := secrets.PersistMasterKeyFile(path, out); err != nil {
		return err
	}
	if *unwrap {
		_, _ = fmt.Fprintf(stdout, "master key at %s is plain again\n", path)
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "master key at %s is now wrapped by %s. Restart the control plane with the same %s settings.\n", path, w.Name(), envKMSProvider)
	return nil
}
