package store

import "testing"

func TestCanaryRelease(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	if _, ok, err := db.GetCanaryRelease(ctx, "web"); err != nil || ok {
		t.Fatalf("missing row ok=%v err=%v", ok, err)
	}
	c := CanaryRelease{ServiceName: "web", CanaryService: "web--canary", Image: "nginx:2", Weight: 10}
	if err := db.SetCanaryRelease(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Weight = 40
	if err := db.SetCanaryRelease(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetCanaryRelease(ctx, "web")
	if err != nil || !ok || got.Weight != 40 || got.CanaryService != "web--canary" || got.CreatedAt.IsZero() {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
	list, err := db.ListCanaryReleases(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteCanaryRelease(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetCanaryRelease(ctx, "web"); ok {
		t.Fatal("row survived delete")
	}
}
