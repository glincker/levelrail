package runtimesettings

import (
	"errors"
	"testing"
)

func TestValidateForWrite(t *testing.T) {
	isValidation := func(e error) bool {
		var verr *ValidationError
		return errors.As(e, &verr)
	}
	is := func(target error) func(error) bool { return func(e error) bool { return errors.Is(e, target) } }
	tests := []struct {
		name    string
		key     string
		val     string
		opts    WriteOptions
		want    string
		wantErr func(error) bool
	}{
		{"bool ok", KeyAuditSensitiveReads, `true`, WriteOptions{}, "true", nil},
		{"bool string canonical", KeyAuditSensitiveReads, `"on"`, WriteOptions{}, "true", nil},
		{"type mismatch", KeyAuditSensitiveReads, `1`, WriteOptions{}, "", isValidation},
		{"null value", KeyAuditSensitiveReads, `null`, WriteOptions{}, "", isValidation},
		{"duration out of range", KeyWebhookReplayWindow, `"0s"`, WriteOptions{}, "", isValidation},
		{"list rule", KeyBindAllowedRoots, `["/"]`, WriteOptions{}, "", isValidation},
		{"unknown key", "nope", `true`, WriteOptions{}, "", is(ErrUnknownKey)},
		{"weakening unconfirmed", KeyAuditSensitiveReads, `false`, WriteOptions{}, "", is(ErrWeakeningNotConfirmed)},
		{"weakening confirmed", KeyAuditSensitiveReads, `false`, WriteOptions{ConfirmWeakening: true}, "false", nil},
		{"invalid beats weakening", KeyAuditSensitiveReads, `"junk"`, WriteOptions{}, "", isValidation},
		{"sensitive allowed for operator", KeyAuditAnchorWebhookSecret, `"s"`, WriteOptions{}, "s", nil},
		{"sensitive refused", KeyAuditAnchorWebhookSecret, `"s"`, WriteOptions{RefuseSensitive: true}, "", is(ErrSensitiveKey)},
		{"sensitive refused before parse", KeyAuditAnchorWebhookSecret, `1`, WriteOptions{RefuseSensitive: true, ConfirmWeakening: true}, "", is(ErrSensitiveKey)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := New(newFakeStore(), envOf(nil))
			got, err := svc.ValidateForWrite(t.Context(), tc.key, raw(tc.val), tc.opts)
			if tc.wantErr == nil {
				if err != nil || got != tc.want {
					t.Fatalf("got %q, %v; want %q", got, err, tc.want)
				}
				return
			}
			if !tc.wantErr(err) {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}
