package store

import (
	"context"
	"errors"
	"testing"
)

func TestSetModelSwapGroup(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("a", "")); err != nil {
		t.Fatalf("SaveModel: %v", err)
	}

	if err := db.SetModelSwapGroup(ctx, "a", "gpu0"); err != nil {
		t.Fatalf("SetModelSwapGroup: %v", err)
	}
	got, err := db.GetModel(ctx, "a")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if got.SwapGroup != "gpu0" {
		t.Errorf("SwapGroup = %q, want gpu0", got.SwapGroup)
	}

	if err := db.SetModelSwapGroup(ctx, "a", ""); err != nil {
		t.Fatalf("SetModelSwapGroup clear: %v", err)
	}
	got, err = db.GetModel(ctx, "a")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if got.SwapGroup != "" {
		t.Errorf("SwapGroup = %q, want empty after clear", got.SwapGroup)
	}
}

func TestSetModelSwapGroup_NotFound(t *testing.T) {
	db := openTestDB(t)
	if err := db.SetModelSwapGroup(context.Background(), "ghost", "gpu0"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("error = %v, want ErrModelNotFound", err)
	}
}

func TestListModelsInSwapGroup(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		if err := db.SaveModel(ctx, testModel(name, "")); err != nil {
			t.Fatalf("SaveModel(%q): %v", name, err)
		}
	}
	if err := db.SetModelSwapGroup(ctx, "a", "gpu0"); err != nil {
		t.Fatalf("SetModelSwapGroup(a): %v", err)
	}
	if err := db.SetModelSwapGroup(ctx, "b", "gpu0"); err != nil {
		t.Fatalf("SetModelSwapGroup(b): %v", err)
	}

	got, err := db.ListModelsInSwapGroup(ctx, "gpu0")
	if err != nil {
		t.Fatalf("ListModelsInSwapGroup: %v", err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("got = %+v, want [a, b]", got)
	}
}

func TestListModelsInSwapGroup_EmptyGroupNameReturnsNothing(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListModelsInSwapGroup(context.Background(), "")
	if err != nil {
		t.Fatalf("ListModelsInSwapGroup: %v", err)
	}
	if got != nil {
		t.Fatalf("got = %+v, want nil", got)
	}
}

func TestListModelsInSwapGroup_NoMembers(t *testing.T) {
	db := openTestDB(t)
	got, err := db.ListModelsInSwapGroup(context.Background(), "gpu0")
	if err != nil {
		t.Fatalf("ListModelsInSwapGroup: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty", got)
	}
}
