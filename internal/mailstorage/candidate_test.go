package mailstorage

import (
	"context"
	"errors"
	"testing"
)

type countingStore struct{ sets int }

func (c *countingStore) SetValue(context.Context, string, string, string) error {
	c.sets++
	return nil
}
func (c *countingStore) Resolve(context.Context, string, string) (string, error) {
	return "", errors.New("unused")
}
func (c *countingStore) DeleteValue(context.Context, string, string) error { return nil }

func TestCandidateDoesNotTouchTheSharedSecretSlot(t *testing.T) {
	spec := Spec{ServiceID: "mail", Bucket: "b", AccessKeyID: "k", Secret: "s"}
	c, err := Candidate(spec)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := c.config(context.Background())
	if err != nil || cfg.SecretAccessKey != "s" {
		t.Fatalf("candidate config = %v", err)
	}
	if err := c.Forget(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := &countingStore{}
	if _, err := New(context.Background(), st, spec); err != nil || st.sets != 1 {
		t.Fatalf("New must seal exactly once: sets=%d err=%v", st.sets, err)
	}
}
