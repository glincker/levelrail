package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func seedAssetTarget(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveBackupTarget(ctx, BackupTarget{ID: "tgt1", Name: "t", Provider: BackupProviderCustom, Endpoint: "https://s3.example.com", Region: "garage", Bucket: "b", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
		t.Fatalf("SaveBackupTarget: %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "p1", Name: "p1", CreatedAt: "2026-10-25T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
}

func TestAsset_BucketPurposeDefaultsToApp(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedAssetTarget(t, db)
	if _, err := db.CreateStorageBucket(ctx, StorageBucket{ID: "b1", Name: "app-one", TargetID: "tgt1", Prefix: "a/"}); err != nil {
		t.Fatalf("CreateStorageBucket: %v", err)
	}
	got, err := db.GetStorageBucket(ctx, "b1")
	if err != nil || got.Purpose != StorageBucketPurposeApp {
		t.Fatalf("GetStorageBucket = %+v, %v; want purpose app", got, err)
	}
	if _, err := db.GetAssetBucket(ctx, "b1"); !errors.Is(err, ErrAssetBucketNotFound) {
		t.Fatalf("GetAssetBucket on app bucket err = %v, want ErrAssetBucketNotFound", err)
	}
}

func TestAsset_BucketLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedAssetTarget(t, db)
	if _, err := db.CreateStorageBucket(ctx, StorageBucket{ID: "b0", Name: "app-one", TargetID: "tgt1", Prefix: "a/"}); err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateAssetBucket(ctx, StorageBucket{ID: "b1", Name: "media", TargetID: "tgt1"}, AssetBucketSettings{ProjectID: "p1", ReferrerAllow: []string{"example.com"}})
	if err != nil {
		t.Fatalf("CreateAssetBucket: %v", err)
	}
	if created.Settings.Visibility != AssetVisibilityPrivate || created.Settings.ServingMode != AssetServingDirect {
		t.Fatalf("defaults = %+v, want private and direct", created.Settings)
	}
	if _, err := db.CreateAssetBucket(ctx, StorageBucket{ID: "b2", Name: "media", TargetID: "tgt1"}, AssetBucketSettings{}); !errors.Is(err, ErrStorageBucketExists) {
		t.Fatalf("duplicate name err = %v, want ErrStorageBucketExists", err)
	}
	if _, err := db.CreateAssetBucket(ctx, StorageBucket{ID: "b3", Name: "bad", TargetID: "tgt1"}, AssetBucketSettings{ServingMode: "teleport"}); !errors.Is(err, ErrAssetBucketInvalid) {
		t.Fatalf("bad mode err = %v, want ErrAssetBucketInvalid", err)
	}

	got, err := db.GetAssetBucket(ctx, "b1")
	if err != nil {
		t.Fatalf("GetAssetBucket: %v", err)
	}
	if got.Bucket.Purpose != StorageBucketPurposeAssets || got.Settings.ProjectID != "p1" || !reflect.DeepEqual(got.Settings.ReferrerAllow, []string{"example.com"}) {
		t.Fatalf("GetAssetBucket = %+v", got)
	}

	s := got.Settings
	s.Visibility, s.PublicBaseURL, s.RefuseNoReferer, s.ReferrerAllow = AssetVisibilityPublic, "https://assets.example.com", true, nil
	if err := db.UpdateAssetBucketSettings(ctx, s); err != nil {
		t.Fatalf("UpdateAssetBucketSettings: %v", err)
	}
	got, _ = db.GetAssetBucket(ctx, "b1")
	if got.Settings.Visibility != AssetVisibilityPublic || !got.Bucket.Public || !got.Settings.RefuseNoReferer || len(got.Settings.ReferrerAllow) != 0 {
		t.Fatalf("after update = %+v", got)
	}
	if err := db.UpdateAssetBucketSettings(ctx, AssetBucketSettings{BucketID: "nope"}); !errors.Is(err, ErrAssetBucketNotFound) {
		t.Fatalf("update missing err = %v", err)
	}

	all, err := db.ListAssetBuckets(ctx, "")
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAssetBuckets = %d, %v; want 1 (app bucket excluded)", len(all), err)
	}
	if other, _ := db.ListAssetBuckets(ctx, "p2"); len(other) != 0 {
		t.Fatalf("project filter returned %d, want 0", len(other))
	}
	byPurpose, _ := db.ListStorageBucketsByPurpose(ctx, StorageBucketPurposeApp)
	if len(byPurpose) != 1 || byPurpose[0].ID != "b0" {
		t.Fatalf("ListStorageBucketsByPurpose(app) = %+v", byPurpose)
	}

	if err := db.DeleteStorageBucket(ctx, "b1"); err != nil {
		t.Fatalf("DeleteStorageBucket: %v", err)
	}
	if _, err := db.GetAssetBucket(ctx, "b1"); !errors.Is(err, ErrAssetBucketNotFound) {
		t.Fatalf("after delete err = %v", err)
	}
}

func TestAsset_IndexUpsertListDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedAssetTarget(t, db)
	if _, err := db.CreateAssetBucket(ctx, StorageBucket{ID: "b1", Name: "media", TargetID: "tgt1"}, AssetBucketSettings{}); err != nil {
		t.Fatal(err)
	}
	first, err := db.UpsertAsset(ctx, Asset{BucketID: "b1", Key: "img/a.png", SizeBytes: 10, ContentType: "image/png", ETag: "e1", Uploader: "tok1"})
	if err != nil {
		t.Fatalf("UpsertAsset: %v", err)
	}
	again, err := db.UpsertAsset(ctx, Asset{BucketID: "b1", Key: "img/a.png", SizeBytes: 25, ContentType: "image/png", ETag: "e2", Uploader: "tok2"})
	if err != nil {
		t.Fatal(err)
	}
	if again.SizeBytes != 25 || again.ETag != "e2" || again.Uploader != "tok1" || again.CreatedAt != first.CreatedAt {
		t.Fatalf("upsert = %+v, want size and etag updated, uploader and created kept", again)
	}
	if _, err := db.UpsertAsset(ctx, Asset{BucketID: "b1", Key: "img/b.png", SizeBytes: 5}); err != nil {
		t.Fatal(err)
	}

	page, err := db.ListAssets(ctx, "b1", "", 1)
	if err != nil || len(page) != 1 || page[0].Key != "img/a.png" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	page, _ = db.ListAssets(ctx, "b1", "img/a.png", 10)
	if len(page) != 1 || page[0].Key != "img/b.png" {
		t.Fatalf("second page = %+v", page)
	}
	n, size, err := db.AssetTotals(ctx, "b1")
	if err != nil || n != 2 || size != 30 {
		t.Fatalf("AssetTotals = %d, %d, %v; want 2, 30", n, size, err)
	}

	if err := db.DeleteAsset(ctx, "b1", "img/a.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetAsset(ctx, "b1", "img/a.png"); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("GetAsset after delete err = %v", err)
	}
	if err := db.DeleteAsset(ctx, "b1", "missing"); err != nil {
		t.Fatalf("DeleteAsset missing = %v, want nil", err)
	}

	if err := db.DeleteStorageBucket(ctx, "b1"); err != nil {
		t.Fatal(err)
	}
	if n, _, _ := db.AssetTotals(ctx, "b1"); n != 0 {
		t.Fatalf("index rows survived bucket delete: %d", n)
	}
}
