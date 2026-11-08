package platformimport

import (
	"errors"
	"strings"
	"testing"
)

func TestImportBoundsReadBoundedRefusesOversize(t *testing.T) {
	SetLimits(100, 3)
	t.Cleanup(func() { SetLimits(0, 0) })
	if _, err := ReadBounded(strings.NewReader(strings.Repeat("x", 100)), "body"); err != nil {
		t.Fatalf("at the cap must pass: %v", err)
	}
	_, err := ReadBounded(strings.NewReader(strings.Repeat("x", 101)), "body")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestImportBoundsItemCapAndDefaults(t *testing.T) {
	SetLimits(0, 2)
	t.Cleanup(func() { SetLimits(0, 0) })
	if err := CheckItems(2, "list"); err != nil {
		t.Fatal(err)
	}
	if err := CheckItems(3, "list"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	SetLimits(0, 0)
	if MaxBytes() != maxResponseBytes || MaxItems() != defaultMaxItems {
		t.Fatal("zero must restore defaults")
	}
}
