package build

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	bkclient "github.com/moby/buildkit/client"
)

// Cache export modes accepted by BuildKit's s3 backend.
const (
	CacheModeMin = "min"
	CacheModeMax = "max"
)

const s3CacheName = "cache"

var errS3CacheIncomplete = errors.New("build: s3 cache needs a bucket and credentials")

// S3Cache configures BuildKit's s3 cache backend. It carries credentials, so
// it must never be logged or serialised; String is safe.
type S3Cache struct {
	Region          string
	Bucket          string
	Prefix          string
	Endpoint        string
	PathStyle       bool
	AccessKeyID     string
	SecretAccessKey string
	Mode            string
}

func (s S3Cache) validate() error {
	if s.Bucket == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return errS3CacheIncomplete
	}
	if s.Mode != "" && s.Mode != CacheModeMin && s.Mode != CacheModeMax {
		return fmt.Errorf("build: s3 cache mode %q must be min or max", s.Mode)
	}
	return nil
}

func (s S3Cache) attrs() map[string]string {
	prefix := s.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	attrs := map[string]string{
		"bucket":            s.Bucket,
		"prefix":            prefix,
		"name":              s3CacheName,
		"access_key_id":     s.AccessKeyID,
		"secret_access_key": s.SecretAccessKey,
		"use_path_style":    strconv.FormatBool(s.PathStyle),
	}
	if s.Region != "" {
		attrs["region"] = s.Region
	}
	if s.Endpoint != "" {
		attrs["endpoint_url"] = s.Endpoint
	}
	return attrs
}

func (s S3Cache) entries() (imports, exports []bkclient.CacheOptionsEntry, err error) {
	if err := s.validate(); err != nil {
		return nil, nil, err
	}
	mode := s.Mode
	if mode == "" {
		mode = CacheModeMax
	}
	expAttrs := s.attrs()
	expAttrs["mode"] = mode
	return []bkclient.CacheOptionsEntry{{Type: "s3", Attrs: s.attrs()}},
		[]bkclient.CacheOptionsEntry{{Type: "s3", Attrs: expAttrs}}, nil
}

// String describes the cache without any credential.
func (s S3Cache) String() string {
	return fmt.Sprintf("s3 bucket=%s prefix=%s mode=%s", s.Bucket, s.Prefix, s.Mode)
}
