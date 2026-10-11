package proxyroutes

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// NotWritableCode is the error code reported when the directory cannot be
// written by this process.
const NotWritableCode = "not_writable"

// NotWritableError explains why this process cannot write the directory.
type NotWritableError struct {
	Dir    string
	Reason string
}

func (e *NotWritableError) Error() string {
	return NotWritableCode + ": " + e.Reason
}

// CheckWritable creates and removes a probe file in the directory, the only
// reliable test under a sandbox, a read-only mount or another user.
func (d *Dir) CheckWritable() error {
	if err := d.recheck(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.real, "."+d.ns.FilePrefix()+"probe-*.tmp")
	if err != nil {
		return &NotWritableError{Dir: d.path, Reason: notWritableReason(d.path, err)}
	}
	name := tmp.Name()
	_ = tmp.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove write probe in %s: %w", d.path, err)
	}
	return nil
}

func notWritableReason(dir string, err error) string {
	uid, gid := os.Geteuid(), os.Getegid()
	owner := ""
	if fi, statErr := os.Stat(dir); statErr == nil {
		if st, ok := fi.Sys().(*syscall.Stat_t); ok {
			owner = fmt.Sprintf(", the directory is owned by uid %d gid %d with mode %s", st.Uid, st.Gid, fi.Mode().Perm())
		}
	}
	switch {
	case errors.Is(err, syscall.EROFS):
		return fmt.Sprintf("%s is on a read-only file system for this process (a systemd sandbox such as ProtectSystem, or a read-only mount): allow it with ReadWritePaths=%s", dir, dir)
	case errors.Is(err, os.ErrPermission):
		return fmt.Sprintf("this process runs as uid %d gid %d and may not create files in %s%s", uid, gid, dir, owner)
	default:
		return fmt.Sprintf("cannot create a file in %s: %v", dir, err)
	}
}
