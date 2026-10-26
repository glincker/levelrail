package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// envAssetsDirectMaxBytes is the size above which upload skips the control plane and PUTs to the provider.
const envAssetsDirectMaxBytes = "LEVELRAIL_ASSETS_DIRECT_MAX_BYTES"

func assetsUsageCmd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var egress *float64
	c, pos, code, ok := assetsCall(prog, "usage", "<bucket> [--egress-gb N]", "\n  --egress-gb N   monthly egress in GB to price (default: none)\n\nThe cost is an estimate from list prices, not an invoice.\n", 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		egress = fs.Float64("egress-gb", -1, "monthly egress in GB to price")
	})
	if !ok {
		return code
	}
	if flagWasSet(c.fs, "egress-gb") && *egress < 0 {
		return c.invalid("--egress-gb must not be negative")
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	u, raw, err := c.client.AssetBucketUsage(ctx, b.ID, *egress)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssetUsage(c.stdout, b.Name, u) })
}

func assetsLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var prefix, cursor *string
	var limit *int
	c, pos, code, ok := assetsCall(prog, "ls", "<bucket> [--prefix P] [--cursor C] [--limit N]", "\n  --prefix P   only keys that start with P\n  --cursor C   continue after a previous page\n  --limit N    page size\n", 1, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		prefix = fs.String("prefix", "", "only keys that start with this prefix")
		cursor = fs.String("cursor", "", "continue after a previous page")
		limit = fs.Int("limit", 0, "page size")
	})
	if !ok {
		return code
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	page, raw, err := c.client.ListAssets(ctx, b.ID, *prefix, *cursor, *limit)
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssets(c.stdout, page) })
}

func assetsURL(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var expires *time.Duration
	c, pos, code, ok := assetsCall(prog, "url", "<bucket> <key> [--expires D]", "\n  --expires D   lifetime of a presigned URL, such as 15m or 2h (the server caps it)\n", 2, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		expires = fs.Duration("expires", 0, "lifetime of a presigned URL")
	})
	if !ok {
		return code
	}
	if *expires < 0 || (*expires > 0 && *expires < time.Second) {
		return c.invalid("--expires must be at least 1s")
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	u, raw, err := c.client.AssetURLs(ctx, b.ID, pos[1], int(*expires/time.Second))
	if err != nil {
		return c.fail(err)
	}
	return c.render(raw, func() { printAssetURLs(c.stdout, u) })
}

func assetsRm(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, pos, code, ok := assetsCall(prog, "rm", "<bucket> <key>", "", 2, args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	if err := c.client.DeleteAsset(ctx, b.ID, pos[1]); err != nil {
		return c.fail(err)
	}
	return c.render(map[string]string{"deleted": pos[1]}, func() { _, _ = fmt.Fprintf(c.stdout, "deleted %s\n", pos[1]) })
}

// assetUploadResult is the --json shape of an upload: the indexed asset plus its URLs.
type assetUploadResult struct {
	Asset json.RawMessage `json:"asset"`
	URLs  json.RawMessage `json:"urls,omitempty"`
}

func assetsUpload(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var key, contentType *string
	c, pos, code, ok := assetsCall(prog, "upload", "<bucket> <file> [--key K] [--content-type T]", "\n  --key K            object key (default: the file name)\n  --content-type T   media type (default: from the key's extension)\n\nSmall files go through the control plane. Files above "+envAssetsDirectMaxBytes+", or\nrefused with 413, are sent straight to the provider with a presigned URL.\n", 2, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		key = fs.String("key", "", "object key (default: the file name)")
		contentType = fs.String("content-type", "", "media type (default: from the key's extension)")
	})
	if !ok {
		return code
	}
	threshold, err := directMaxBytes(lookupEnv)
	if err != nil {
		return c.fail(err)
	}
	info, err := os.Stat(pos[1])
	if err != nil {
		return c.fail(newValidationError("cannot read %s: %v", pos[1], err))
	}
	if !info.Mode().IsRegular() {
		return c.invalid("%s is not a regular file", pos[1])
	}
	objKey := *key
	if objKey == "" {
		objKey = filepath.Base(pos[1])
	}
	ctype := *contentType
	if ctype == "" {
		ctype = mime.TypeByExtension(path.Ext(objKey))
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	ctx := context.Background()
	b, err := resolveAssetBucket(ctx, c.client, pos[0])
	if err != nil {
		return c.fail(err)
	}
	size := info.Size()
	asset, raw, err := uploadAsset(ctx, c.client, b.ID, pos[1], objKey, ctype, size, threshold)
	if err != nil {
		return c.fail(err)
	}
	urls, urlsRaw, urlErr := c.client.AssetURLs(ctx, b.ID, asset.Key, 0)
	if urlErr != nil {
		_, _ = fmt.Fprintf(c.stderr, "uploaded, but could not look up the URLs: %v\n", urlErr)
	}
	return c.render(assetUploadResult{Asset: raw, URLs: urlsRaw}, func() {
		_, _ = fmt.Fprintf(c.stdout, "uploaded %s (%s)\n", asset.Key, humanBytes(size))
		if urlErr == nil {
			printUploadURLs(c.stdout, b.Name, urls)
		}
	})
}

// directMaxBytes reads the optional size cutoff; unset means try the direct route and fall back on 413.
func directMaxBytes(lookupEnv func(string) (string, bool)) (int64, error) {
	v, _ := lookupEnv(envAssetsDirectMaxBytes)
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, newValidationError("%s must be a non-negative number of bytes, got %q", envAssetsDirectMaxBytes, v)
	}
	return n, nil
}

func uploadAsset(ctx context.Context, client *apiclient.Client, bucketID, file, key, ctype string, size, threshold int64) (apiclient.Asset, json.RawMessage, error) {
	if threshold > 0 && size > threshold {
		return uploadAssetPresigned(ctx, client, bucketID, file, key, ctype, size)
	}
	f, err := os.Open(file) //nolint:gosec // the operator chose this path
	if err != nil {
		return apiclient.Asset{}, nil, err
	}
	asset, raw, err := client.UploadAssetDirect(ctx, bucketID, key, ctype, size, f)
	_ = f.Close()
	var apiErr *apiError
	if err != nil && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusRequestEntityTooLarge {
		return uploadAssetPresigned(ctx, client, bucketID, file, key, ctype, size)
	}
	return asset, raw, err
}

// uploadAssetPresigned sends the bytes to the provider with the exact size the URL was signed for, then confirms.
func uploadAssetPresigned(ctx context.Context, client *apiclient.Client, bucketID, file, key, ctype string, size int64) (apiclient.Asset, json.RawMessage, error) {
	up, _, err := client.RequestAssetUpload(ctx, bucketID, apiclient.AssetUploadRequest{Key: key, ContentType: ctype, Size: size})
	if err != nil {
		return apiclient.Asset{}, nil, err
	}
	f, err := os.Open(file) //nolint:gosec // the operator chose this path
	if err != nil {
		return apiclient.Asset{}, nil, err
	}
	defer func() { _ = f.Close() }()
	if err := client.PutPresigned(ctx, up, size, f); err != nil {
		return apiclient.Asset{}, nil, err
	}
	return client.ConfirmAssetUpload(ctx, bucketID, key)
}
