package build

import (
	"testing"
	"time"

	bkclient "github.com/moby/buildkit/client"
)

func rec(id, desc string, size int64, last time.Time) *bkclient.UsageInfo {
	return &bkclient.UsageInfo{ID: id, Description: desc, Size: size, RecordType: "exec.cachemount", LastUsedAt: &last}
}

func TestAppCacheRecordsPrefixAndSiblings(t *testing.T) {
	now := time.Now()
	recs := []*bkclient.UsageInfo{
		rec("a", `cached mount /app/.next/cache from buildkit with id "web-next-cache"`, 100, now),
		rec("b", `cached mount /root from buildkit with id "web-api-npm"`, 50, now),
		rec("c", `cached mount /root from buildkit with id "other-npm"`, 70, now),
		{ID: "d", Description: `cached mount /x with id "web-x"`, RecordType: "regular", Size: 9},
	}
	got := AppCacheRecords(recs, "web", []string{"web", "web-api"})
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("got %+v", got)
	}
	if u := SummarizeAppCache(got); u.Bytes != 100 || u.Records != 1 || u.LastUsedAt.IsZero() {
		t.Fatalf("usage %+v", u)
	}
	if AppCacheRecords(recs, "", nil) != nil {
		t.Fatal("empty id must match nothing")
	}
}
