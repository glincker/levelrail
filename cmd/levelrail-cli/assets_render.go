package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func assetDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printAssetBuckets(w io.Writer, list []apiclient.AssetBucket) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(w, "no asset buckets")
		return
	}
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "NAME\tVISIBILITY\tSERVING\tPRESET\tPREFIX\tID")
	for _, b := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", b.Name, b.Visibility, b.ServingMode, assetDash(b.Preset), assetDash(b.Prefix), b.ID)
	}
	_ = tw.Flush()
}

func printAssetBucket(w io.Writer, b apiclient.AssetBucket) {
	quota := "none"
	if b.QuotaBytes > 0 {
		quota = humanBytes(b.QuotaBytes)
	}
	_, _ = fmt.Fprintf(w, "name:         %s\nid:           %s\nvisibility:   %s\nserving:      %s\ntarget:       %s (%s)\nprefix:       %s\nproject:      %s\npublic URL:   %s\nCDN URL:      %s\nreferrers:    %s\nquota:        %s\n",
		b.Name, b.ID, b.Visibility, b.ServingMode, b.TargetID, assetDash(b.Preset), assetDash(b.Prefix), assetDash(b.ProjectID),
		assetDash(b.PublicBaseURL), assetDash(b.CDNBaseURL), assetDash(strings.Join(b.ReferrerAllow, ", ")), quota)
}

func printAssetProbe(w io.Writer, p apiclient.AssetProbe) {
	if p.OK {
		_, _ = fmt.Fprintln(w, "probe passed")
	} else {
		_, _ = fmt.Fprintf(w, "probe failed: %s\n", assetDash(p.Message))
	}
	for _, s := range p.Steps {
		mark := "ok"
		if !s.OK {
			mark = "FAILED " + s.Error
		}
		_, _ = fmt.Fprintf(w, "  %-10s %s\n", s.Name, strings.TrimSpace(mark))
	}
	if p.CORS != nil {
		_, _ = fmt.Fprintf(w, "cors: %s for %s %s\n", p.CORS.Status, assetDash(p.CORS.Origin), p.CORS.Message)
	}
	if p.Exposure != nil {
		_, _ = fmt.Fprintf(w, "exposure: %s anonymous read=%t status=%d %s\n", p.Exposure.URL, p.Exposure.Public, p.Exposure.Status, p.Exposure.Error)
	}
	for _, wn := range p.Warnings {
		_, _ = fmt.Fprintf(w, "warning: %s\n", wn.Message)
		if wn.Fix == "fix_cors" || strings.Contains(wn.Fix, "cors") {
			_, _ = fmt.Fprintln(w, "  fix: rerun with --fix-cors")
		}
	}
}

func printAssetUsage(w io.Writer, name string, u apiclient.AssetUsage) {
	count := fmt.Sprint(u.Objects)
	size := humanBytes(u.Bytes)
	if u.AtLeast {
		count, size = "at least "+count, "at least "+size
	}
	quota := "none"
	if u.QuotaBytes > 0 {
		quota = humanBytes(u.QuotaBytes)
	}
	_, _ = fmt.Fprintf(w, "bucket:   %s\nobjects:  %s\nstored:   %s\nquota:    %s\nserving:  %s\n", name, count, size, quota, u.ServingMode)
	c := u.Cost
	label := "Cost"
	if c.Estimated {
		label = "Estimated cost"
	}
	_, _ = fmt.Fprintf(w, "\n%s per month (list prices as of %s, not your invoice), egress %.1f GB:\n", label, assetDash(c.PriceTableDate), c.EgressGB)
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "PRESET\tSTORAGE\tEGRESS\tTOTAL")
	for _, l := range append([]apiclient.AssetCostLine{c.Current}, c.Compare...) {
		_, _ = fmt.Fprintf(tw, "%s\t$%.2f\t$%.2f\t$%.2f\n", l.Preset, l.StorageUSD, l.EgressUSD, l.TotalUSD)
	}
	_ = tw.Flush()
	for _, n := range c.Notes {
		_, _ = fmt.Fprintf(w, "note: %s\n", n)
	}
}

func printAssets(w io.Writer, page apiclient.AssetList) {
	if len(page.Assets) == 0 {
		_, _ = fmt.Fprintln(w, "no objects")
		return
	}
	tw := newStackTable(w)
	_, _ = fmt.Fprintln(tw, "KEY\tSIZE\tTYPE\tMODIFIED")
	for _, a := range page.Assets {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.Key, humanBytes(a.Size), assetDash(a.ContentType), assetDash(a.LastModified))
	}
	_ = tw.Flush()
	if page.Cursor != "" {
		_, _ = fmt.Fprintf(w, "more: --cursor %s\n", page.Cursor)
	}
}

func printAssetURLs(w io.Writer, u apiclient.AssetURLs) {
	if u.OriginURL != "" {
		_, _ = fmt.Fprintf(w, "origin URL: %s\n", u.OriginURL)
	}
	if u.CDNURL != "" {
		_, _ = fmt.Fprintf(w, "CDN URL:    %s\n", u.CDNURL)
	}
	if u.PresignedURL != "" {
		_, _ = fmt.Fprintf(w, "presigned URL (expires %s): %s\n", assetDash(u.PresignedExpires), u.PresignedURL)
	}
	for _, n := range u.Notes {
		_, _ = fmt.Fprintf(w, "note: %s\n", n)
	}
}

// printUploadURLs shows only the stable URLs after an upload, never a presigned one.
func printUploadURLs(w io.Writer, bucket string, u apiclient.AssetURLs) {
	if u.OriginURL == "" && u.CDNURL == "" {
		_, _ = fmt.Fprintf(w, "no public URL: run 'assets url %s %s' for a time-limited one\n", bucket, u.Key)
		return
	}
	if u.OriginURL != "" {
		_, _ = fmt.Fprintf(w, "origin URL: %s\n", u.OriginURL)
	}
	if u.CDNURL != "" {
		_, _ = fmt.Fprintf(w, "CDN URL:    %s\n", u.CDNURL)
	}
}
