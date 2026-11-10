package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// importHeaderProbeBytes covers the longest marker below (the armored age line).
const importHeaderProbeBytes = 64

var (
	ageBinaryHeader  = []byte("age-encryption.org/")
	ageArmoredHeader = []byte("-----BEGIN AGE ENCRYPTED FILE-----")
	zstdFrameMagic   = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

const (
	msgImportEncrypted   = "this file is an encrypted export and cannot be imported as is; import a plain dump instead (for example from pg_dump, mysqldump or mongodump --archive)"
	msgImportKeyMismatch = "this export was sealed with a different instance key; import it on the instance that made it, or import a plain dump"
	msgImportUnsealFail  = "the export could not be decrypted; nothing was restored"
	msgImportCompressed  = "this file is a compressed export; decompress it first (zstd -d) and import the plain dump"
)

// sealedImportReason reads the start of the spooled upload and rewinds it.
// The restorers feed bytes straight to the engine client, so a sealed
// object would fail there and echo binary fragments back in stderr.
func sealedImportReason(f io.ReadSeeker, engine string) (string, error) {
	head := make([]byte, importHeaderProbeBytes)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	head = bytes.TrimLeft(head[:n], " \t\r\n")
	switch {
	case bytes.HasPrefix(head, ageBinaryHeader), bytes.HasPrefix(head, ageArmoredHeader):
		return msgImportEncrypted, nil
	case bytes.HasPrefix(head, zstdFrameMagic) && !store.IsSearchEngine(engine):
		return msgImportCompressed, nil
	}
	return "", nil
}

// sealedExportOpener is the optional protection surface that decrypts an export with the instance key.
type sealedExportOpener interface {
	OpenSealedExport(r io.Reader) (io.ReadCloser, error)
}

// isAgeExport reports whether the spool starts with the binary age header the exports carry.
func isAgeExport(f io.ReadSeeker) (bool, error) {
	head := make([]byte, len(ageBinaryHeader))
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	return bytes.Equal(head[:n], ageBinaryHeader), nil
}

// unsealImport decrypts the spooled export into a new spool bounded by maxBytes, so a
// compressed bomb cannot outgrow the import limit. The input spool is discarded.
func (rt *Router) unsealImport(in importSpool, maxBytes int64) (importSpool, int64, string) {
	defer in.discard()
	opener, ok := rt.backupProtection.(sealedExportOpener)
	if !ok || rt.backupProtection == nil {
		return importSpool{}, 0, msgImportEncrypted
	}
	plain, err := opener.OpenSealedExport(in.f)
	if err != nil {
		if errors.Is(err, backup.ErrExportKeyMismatch) {
			return importSpool{}, 0, msgImportKeyMismatch
		}
		return importSpool{}, 0, msgImportUnsealFail
	}
	defer func() { _ = plain.Close() }()
	out, n, err := spoolImportBody(io.LimitReader(plain, maxBytes+1))
	if err != nil {
		return importSpool{}, 0, msgImportUnsealFail
	}
	if n > maxBytes {
		out.discard()
		return importSpool{}, 0, fmt.Sprintf("the decrypted export exceeds the %d MB import limit", maxBytes>>20)
	}
	return out, n, ""
}

// importErrorClass maps a restore failure to a fixed label for the log, so
// engine stderr (which can quote dump lines) never reaches it.
func importErrorClass(err error) (class string, exitStatus int) {
	var exit *docker.ExecExitError
	if !errors.As(err, &exit) {
		return "runtime", -1
	}
	stderr := strings.ToLower(exit.Stderr)
	switch {
	case strings.Contains(stderr, "syntax error"), strings.Contains(stderr, "invalid command"), strings.Contains(stderr, "invalid byte sequence"):
		return "dump_not_readable", exit.ExitCode
	case strings.Contains(stderr, "permission denied"), strings.Contains(stderr, "must be owner"):
		return "permission", exit.ExitCode
	case strings.Contains(stderr, "authentication failed"), strings.Contains(stderr, "access denied"):
		return "auth", exit.ExitCode
	case strings.Contains(stderr, "already exists"), strings.Contains(stderr, "does not exist"):
		return "schema_conflict", exit.ExitCode
	}
	return "engine_exit", exit.ExitCode
}
