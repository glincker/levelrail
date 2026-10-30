package mailstorage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ErrKeyOutsidePrefix is returned for any key that is not under the
// service prefix, including traversal and prefix-sibling tricks.
var ErrKeyOutsidePrefix = errors.New("mailstorage: key is outside the service prefix")

// Prefix is the only key space a service may touch.
func Prefix(serviceID string) string { return serviceID + "/" }

// MailboxHash hides addresses in object listings.
func MailboxHash(address string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(address))))
	return hex.EncodeToString(sum[:])
}

// BackupKey is the key of one backup archive.
func BackupKey(serviceID string, at time.Time) string {
	return Prefix(serviceID) + "backups/" + at.UTC().Format(time.RFC3339) + ".tar.zst.age"
}

// ArchiveKey is the key of one mailbox month archive.
func ArchiveKey(serviceID, mailbox string, month time.Time) string {
	return Prefix(serviceID) + "archive/" + MailboxHash(mailbox) + "/" + month.UTC().Format("2006-01") + ".mbox.zst.age"
}

// ExportKey is the key of one export bundle.
func ExportKey(serviceID, exportID string) string {
	return Prefix(serviceID) + "exports/" + exportID + ".zip.age"
}

// ProbePrefix holds throwaway probe objects.
func ProbePrefix(serviceID string) string { return Prefix(serviceID) + ".probe/" }

// Guard accepts only clean keys strictly inside the service prefix.
func Guard(serviceID, key string) error {
	if !ValidServiceID(serviceID) {
		return ErrKeyOutsidePrefix
	}
	prefix := Prefix(serviceID)
	if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
		return ErrKeyOutsidePrefix
	}
	if strings.ContainsAny(key, "\\\x00") || strings.Contains(key, "//") {
		return ErrKeyOutsidePrefix
	}
	for _, seg := range strings.Split(key[len(prefix):], "/") {
		if seg == ".." || seg == "." {
			return ErrKeyOutsidePrefix
		}
	}
	return nil
}

// guardPrefix is Guard for list prefixes, which may end at the service root.
func guardPrefix(serviceID, prefix string) error {
	if prefix == Prefix(serviceID) {
		return nil
	}
	return Guard(serviceID, prefix)
}
