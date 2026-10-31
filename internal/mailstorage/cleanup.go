package mailstorage

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"
)

// RetentionDaysEnv is how long data survives an "after retention" uninstall.
const RetentionDaysEnv = "APP_MAIL_UNINSTALL_RETENTION_DAYS"

// ErrDeleteNotConfirmed is returned when the confirm token does not match.
var ErrDeleteNotConfirmed = errors.New("mailstorage: delete not confirmed")

// ConfirmToken is what the operator types to authorize wiping a service prefix.
func ConfirmToken(serviceID string) string { return "delete " + serviceID }

// RetentionDays reads the uninstall retention window, default 30.
func RetentionDays() int {
	if n, err := strconv.Atoi(os.Getenv(RetentionDaysEnv)); err == nil && n >= 0 {
		return n
	}
	return 30
}

// DeleteResult reports what cleanup removed.
type DeleteResult struct {
	Deleted int64
	Bytes   int64
}

// DeletePrefix removes the service's objects after an uninstall. With
// afterRetention only objects older than RetentionDays go; otherwise all.
func DeletePrefix(ctx context.Context, conn Connection, confirm string, afterRetention bool) (DeleteResult, error) {
	if !ValidServiceID(conn.ServiceID) || confirm != ConfirmToken(conn.ServiceID) {
		return DeleteResult{}, ErrDeleteNotConfirmed
	}
	st, err := Open(ctx, conn)
	if err != nil {
		return DeleteResult{}, err
	}
	cutoff := time.Now().Add(24 * time.Hour)
	if afterRetention {
		cutoff = time.Now().AddDate(0, 0, -RetentionDays())
	}
	var res DeleteResult
	token := ""
	for {
		page, err := st.List(ctx, Prefix(conn.ServiceID), token, 500)
		if err != nil {
			return res, classify("list", err)
		}
		for _, o := range page.Objects {
			if !o.LastModified.Before(cutoff) {
				continue
			}
			if err := st.Delete(ctx, o.Key); err != nil {
				return res, classify("delete", err)
			}
			res.Deleted++
			res.Bytes += o.Size
		}
		if page.Next == "" {
			return res, nil
		}
		token = page.Next
	}
}
