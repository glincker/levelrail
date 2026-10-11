package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ObjectInfo is what a HEAD request learned about a stored backup object.
type ObjectInfo struct {
	Exists bool
	Size   int64
}

// ObjectProber checks an object without downloading it.
type ObjectProber interface {
	Head(ctx context.Context, dest Destination, key string) (ObjectInfo, error)
}

// Object states CheckObject reports.
const (
	ObjectOK        = "ok"
	ObjectMissing   = "missing"
	ObjectTruncated = "truncated"
)

// CheckObject compares a HEAD result with what was recorded at backup time.
// A recorded size of zero (legacy rows) only requires the object to exist.
func CheckObject(info ObjectInfo, h store.BackupHistory) (state, reason string) {
	switch {
	case !info.Exists:
		return ObjectMissing, fmt.Sprintf("object %q is missing from the bucket", h.ObjectKey)
	case h.SizeBytes > 0 && info.Size < h.SizeBytes:
		return ObjectTruncated, fmt.Sprintf("object is %d bytes but %d were uploaded: it is truncated", info.Size, h.SizeBytes)
	case h.SizeBytes > 0 && info.Size > h.SizeBytes && h.Codec == "":
		return ObjectTruncated, fmt.Sprintf("object is %d bytes but %d were uploaded: it was changed after upload", info.Size, h.SizeBytes)
	}
	return ObjectOK, ""
}

// S3Prober is the real ObjectProber.
type S3Prober struct{}

// Head implements ObjectProber.
func (S3Prober) Head(ctx context.Context, dest Destination, key string) (ObjectInfo, error) {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("backup: head %q: %w", key, err)
	}
	out, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(dest.Bucket), Key: aws.String(key)})
	if err != nil {
		if isNotFound(err) {
			return ObjectInfo{}, nil
		}
		return ObjectInfo{}, fmt.Errorf("backup: head %q in %q: %w", key, dest.Bucket, err)
	}
	return ObjectInfo{Exists: true, Size: aws.ToInt64(out.ContentLength)}, nil
}

func isNotFound(err error) bool {
	var nf *s3types.NotFound
	if errors.As(err, &nf) {
		return true
	}
	var nsk *s3types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return true
		}
	}
	return false
}

// BucketProtection is what a bucket does to protect stored backups.
type BucketProtection struct {
	ObjectLock bool
	LockMode   string
	Versioning string
	CanDelete  bool
}

// Protection levels reported by Level.
const (
	ProtectionLocked  = "locked"
	ProtectionVersion = "versioned"
	ProtectionOpen    = "open"
)

// Level classifies the protection.
func (p BucketProtection) Level() string {
	switch {
	case p.ObjectLock:
		return ProtectionLocked
	case strings.EqualFold(p.Versioning, "Enabled"):
		return ProtectionVersion
	}
	return ProtectionOpen
}

// Warning returns operator-facing advice, or "" when the bucket is locked.
func (p BucketProtection) Warning() string {
	switch p.Level() {
	case ProtectionLocked:
		return ""
	case ProtectionVersion:
		return "Versioning is on but object lock is off: the backup key can still delete old versions. Enable object lock for immutable backups."
	}
	if p.CanDelete {
		return "The backup key can delete or overwrite stored backups and the bucket has neither object lock nor versioning. A leaked key or a bad script can erase every backup. Enable object lock or versioning."
	}
	return "The bucket has neither object lock nor versioning."
}

// ProtectionProber inspects a bucket's protection settings.
type ProtectionProber interface {
	Probe(ctx context.Context, dest Destination) (BucketProtection, error)
}

// S3ProtectionProber is the real ProtectionProber. It writes and deletes one
// small canary object to learn whether the key can delete.
type S3ProtectionProber struct{}

// Probe implements ProtectionProber.
func (S3ProtectionProber) Probe(ctx context.Context, dest Destination) (BucketProtection, error) {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return BucketProtection{}, fmt.Errorf("backup: probe bucket %q: %w", dest.Bucket, err)
	}
	var p BucketProtection

	lock, err := client.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: aws.String(dest.Bucket)})
	if err == nil && lock.ObjectLockConfiguration != nil {
		p.ObjectLock = lock.ObjectLockConfiguration.ObjectLockEnabled == s3types.ObjectLockEnabledEnabled
		if rule := lock.ObjectLockConfiguration.Rule; rule != nil && rule.DefaultRetention != nil {
			p.LockMode = string(rule.DefaultRetention.Mode)
		}
	}

	ver, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(dest.Bucket)})
	if err == nil {
		p.Versioning = string(ver.Status)
	}

	canary := fmt.Sprintf(".protection-probe/%d", time.Now().UnixNano())
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(dest.Bucket), Key: aws.String(canary), Body: bytes.NewReader([]byte("probe"))}); err == nil {
		if _, derr := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(dest.Bucket), Key: aws.String(canary)}); derr == nil {
			p.CanDelete = true
		}
	}
	return p, nil
}
