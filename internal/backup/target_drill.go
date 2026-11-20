package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

// DrillObjectPrefix keeps drill sample objects apart from real backups.
const DrillObjectPrefix = ".drill/"

// ErrDrillUnsupported means the target's storage cannot write and read back objects here.
var ErrDrillUnsupported = errors.New("backup target does not support the restore drill")

// DrillReport is the outcome of writing a sample object and reading it back.
type DrillReport struct {
	OK           bool   `json:"ok"`
	Stage        string `json:"stage,omitempty"`
	Bytes        int    `json:"bytes"`
	UploadMs     int64  `json:"upload_ms"`
	DownloadMs   int64  `json:"download_ms"`
	CleanedUp    bool   `json:"cleaned_up"`
	ChecksumOK   bool   `json:"checksum_ok"`
	FailureError string `json:"error,omitempty"`
}

type drillStorage interface {
	Uploader
	Downloader
	Deleter
}

// DrillTarget uploads a small random object, downloads it, compares the bytes and deletes it.
func (t *TargetTester) DrillTarget(ctx context.Context, targetID string) (DrillReport, error) {
	store, ok := t.Tester.(drillStorage)
	if !ok {
		return DrillReport{}, ErrDrillUnsupported
	}
	dest, err := t.resolve(ctx, targetID)
	if err != nil {
		return DrillReport{}, err
	}
	return runDrill(ctx, store, dest, drillSampleSize), nil
}

// drillSampleSize is a few bytes: enough to prove the round trip without costing storage.
const drillSampleSize = 256

func runDrill(ctx context.Context, st drillStorage, dest Destination, size int) (rep DrillReport) {
	rep = DrillReport{Bytes: size}
	sample := make([]byte, size)
	if _, err := rand.Read(sample); err != nil {
		rep.Stage, rep.FailureError = "prepare", err.Error()
		return rep
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		rep.Stage, rep.FailureError = "prepare", err.Error()
		return rep
	}
	key := DrillObjectPrefix + hex.EncodeToString(nonce)

	start := time.Now()
	if err := st.Upload(ctx, dest, key, bytes.NewReader(sample), int64(size)); err != nil {
		rep.Stage, rep.FailureError = "upload", err.Error()
		return rep
	}
	rep.UploadMs = time.Since(start).Milliseconds()
	defer func() {
		if err := st.Delete(context.WithoutCancel(ctx), dest, key); err == nil {
			rep.CleanedUp = true
		}
	}()

	start = time.Now()
	rc, err := st.Download(ctx, dest, key)
	if err != nil {
		rep.Stage, rep.FailureError = "download", err.Error()
		return rep
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(io.LimitReader(rc, int64(size)+1))
	if err != nil {
		rep.Stage, rep.FailureError = "download", fmt.Sprintf("read sample: %v", err)
		return rep
	}
	rep.DownloadMs = time.Since(start).Milliseconds()
	rep.ChecksumOK = subtle.ConstantTimeCompare(got, sample) == 1
	if !rep.ChecksumOK {
		rep.Stage, rep.FailureError = "compare", "downloaded sample differs from the uploaded one"
		return rep
	}
	rep.OK = true
	return rep
}
