package build

import (
	"context"
	"io"
	"strings"
	"time"
)

const maxCacheWarningLen = 300

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// solveFailOpen runs solve, and when the s3 cache backend is what broke it,
// downgrades to no s3 cache instead of failing the build. A retry is only safe
// while nothing has reached out yet; after that a cache-looking error is
// treated as a cache export failure, and the image tar the loader already
// consumed decides whether the build really succeeded.
func solveFailOpen(ctx context.Context, cache CacheConfig, out io.Writer, progress func(ProgressEvent), solve func(CacheConfig, io.Writer) (*Result, error)) (*Result, error) {
	cw := &countingWriter{w: out}
	start := time.Now()
	res, err := solve(cache, cw)
	if err == nil || cache.S3 == nil || ctx.Err() != nil || !looksLikeCacheError(err) {
		return res, err
	}
	warn := "build cache unavailable, continuing without it: " + cache.S3.redact(err.Error())
	emitCacheWarning(progress, warn)
	if cw.n > 0 {
		return &Result{Duration: time.Since(start)}, nil
	}
	cache.S3 = nil
	return solve(cache, cw)
}

func emitCacheWarning(progress func(ProgressEvent), msg string) {
	if progress == nil {
		return
	}
	progress(ProgressEvent{Log: "warning: " + msg + "\n", Stream: "stderr", CacheWarning: msg})
}

// EmitCacheWarning reports that the remote build cache was skipped.
func EmitCacheWarning(progress func(ProgressEvent), msg string) { emitCacheWarning(progress, msg) }

func looksLikeCacheError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, hint := range []string{"cache", "s3", "accessdenied", "nosuchbucket", "signaturedoesnotmatch", "invalidaccesskey"} {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

func (s S3Cache) redact(msg string) string {
	for _, secret := range []string{s.SecretAccessKey, s.AccessKeyID} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, "***")
		}
	}
	if len(msg) > maxCacheWarningLen {
		msg = msg[:maxCacheWarningLen] + "..."
	}
	return msg
}
