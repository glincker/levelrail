package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testDeviceAuthRequest(id, deviceCode, userCode string) DeviceAuthRequest {
	now := time.Now()
	return DeviceAuthRequest{
		ID:         id,
		DeviceCode: deviceCode,
		UserCode:   userCode,
		ClientName: "test-laptop",
		CreatedAt:  now,
		ExpiresAt:  now.Add(10 * time.Minute),
	}
}

func TestSaveAndGetDeviceAuthRequest(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := testDeviceAuthRequest("dar_1", "device-abc", "WDJB-MJHT")
	if err := db.SaveDeviceAuthRequest(ctx, want); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}

	byDevice, err := db.GetDeviceAuthRequestByDeviceCode(ctx, "device-abc")
	if err != nil {
		t.Fatalf("GetDeviceAuthRequestByDeviceCode() error = %v", err)
	}
	if byDevice.UserCode != "WDJB-MJHT" || byDevice.Status != DeviceAuthStatusPending || byDevice.ClientName != "test-laptop" {
		t.Errorf("got %+v", byDevice)
	}
	if byDevice.ApprovedByUserID != nil || byDevice.TokenID != nil || byDevice.RedeemedAt != nil {
		t.Errorf("got %+v, want all optional fields nil for a fresh request", byDevice)
	}

	byUser, err := db.GetDeviceAuthRequestByUserCode(ctx, "WDJB-MJHT")
	if err != nil {
		t.Fatalf("GetDeviceAuthRequestByUserCode() error = %v", err)
	}
	if byUser.DeviceCode != "device-abc" {
		t.Errorf("got %+v", byUser)
	}
}

func TestGetDeviceAuthRequest_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetDeviceAuthRequestByDeviceCode(ctx, "missing"); !errors.Is(err, ErrDeviceAuthRequestNotFound) {
		t.Errorf("GetDeviceAuthRequestByDeviceCode() error = %v, want ErrDeviceAuthRequestNotFound", err)
	}
	if _, err := db.GetDeviceAuthRequestByUserCode(ctx, "MISSING"); !errors.Is(err, ErrDeviceAuthRequestNotFound) {
		t.Errorf("GetDeviceAuthRequestByUserCode() error = %v, want ErrDeviceAuthRequestNotFound", err)
	}
}

func TestListPendingDeviceAuthRequests(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()

	pending := testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")
	if err := db.SaveDeviceAuthRequest(ctx, pending); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}

	expired := DeviceAuthRequest{
		ID: "dar_2", DeviceCode: "device-2", UserCode: "BBBB-2222",
		CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute),
	}
	if err := db.SaveDeviceAuthRequest(ctx, expired); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}

	approved := testDeviceAuthRequest("dar_3", "device-3", "CCCC-3333")
	if err := db.SaveDeviceAuthRequest(ctx, approved); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}
	if n, err := db.SetDeviceAuthRequestStatus(ctx, "CCCC-3333", DeviceAuthStatusApproved, "user_1"); err != nil || n != 1 {
		t.Fatalf("SetDeviceAuthRequestStatus() = (%d, %v)", n, err)
	}

	got, err := db.ListPendingDeviceAuthRequests(ctx, now)
	if err != nil {
		t.Fatalf("ListPendingDeviceAuthRequests() error = %v", err)
	}
	if len(got) != 1 || got[0].UserCode != "AAAA-1111" {
		t.Errorf("got %+v, want only the still-pending, unexpired request", got)
	}
}

func TestSetDeviceAuthRequestStatus_Approve(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeviceAuthRequest(ctx, testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}

	n, err := db.SetDeviceAuthRequestStatus(ctx, "AAAA-1111", DeviceAuthStatusApproved, "user_1")
	if err != nil || n != 1 {
		t.Fatalf("SetDeviceAuthRequestStatus() = (%d, %v)", n, err)
	}

	got, err := db.GetDeviceAuthRequestByUserCode(ctx, "AAAA-1111")
	if err != nil {
		t.Fatalf("GetDeviceAuthRequestByUserCode() error = %v", err)
	}
	if got.Status != DeviceAuthStatusApproved || got.ApprovedByUserID == nil || *got.ApprovedByUserID != "user_1" {
		t.Errorf("got %+v", got)
	}
}

func TestSetDeviceAuthRequestStatus_Deny(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeviceAuthRequest(ctx, testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}

	n, err := db.SetDeviceAuthRequestStatus(ctx, "AAAA-1111", DeviceAuthStatusDenied, "")
	if err != nil || n != 1 {
		t.Fatalf("SetDeviceAuthRequestStatus() = (%d, %v)", n, err)
	}

	got, err := db.GetDeviceAuthRequestByUserCode(ctx, "AAAA-1111")
	if err != nil {
		t.Fatalf("GetDeviceAuthRequestByUserCode() error = %v", err)
	}
	if got.Status != DeviceAuthStatusDenied || got.ApprovedByUserID != nil {
		t.Errorf("got %+v, want denied with no approver", got)
	}
}

func TestSetDeviceAuthRequestStatus_AlreadyDecided(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeviceAuthRequest(ctx, testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}
	if n, err := db.SetDeviceAuthRequestStatus(ctx, "AAAA-1111", DeviceAuthStatusApproved, "user_1"); err != nil || n != 1 {
		t.Fatalf("first SetDeviceAuthRequestStatus() = (%d, %v)", n, err)
	}

	n, err := db.SetDeviceAuthRequestStatus(ctx, "AAAA-1111", DeviceAuthStatusDenied, "")
	if err != nil {
		t.Fatalf("second SetDeviceAuthRequestStatus() error = %v", err)
	}
	if n != 0 {
		t.Errorf("second SetDeviceAuthRequestStatus() rows affected = %d, want 0 (already-decided request must not flip again)", n)
	}
}

func TestRedeemDeviceAuthRequest(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeviceAuthRequest(ctx, testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}
	if _, err := db.SetDeviceAuthRequestStatus(ctx, "AAAA-1111", DeviceAuthStatusApproved, "user_1"); err != nil {
		t.Fatalf("SetDeviceAuthRequestStatus() error = %v", err)
	}

	redeemedAt := time.Now()
	if err := db.RedeemDeviceAuthRequest(ctx, "device-1", "tok_1", redeemedAt); err != nil {
		t.Fatalf("RedeemDeviceAuthRequest() error = %v", err)
	}

	got, err := db.GetDeviceAuthRequestByDeviceCode(ctx, "device-1")
	if err != nil {
		t.Fatalf("GetDeviceAuthRequestByDeviceCode() error = %v", err)
	}
	if got.TokenID == nil || *got.TokenID != "tok_1" || got.RedeemedAt == nil {
		t.Errorf("got %+v", got)
	}
}

func TestRedeemDeviceAuthRequest_AlreadyRedeemed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDeviceAuthRequest(ctx, testDeviceAuthRequest("dar_1", "device-1", "AAAA-1111")); err != nil {
		t.Fatalf("SaveDeviceAuthRequest() error = %v", err)
	}
	if err := db.RedeemDeviceAuthRequest(ctx, "device-1", "tok_1", time.Now()); err != nil {
		t.Fatalf("first RedeemDeviceAuthRequest() error = %v", err)
	}

	err := db.RedeemDeviceAuthRequest(ctx, "device-1", "tok_2", time.Now())
	if !errors.Is(err, ErrDeviceAuthRequestNotFound) {
		t.Errorf("second RedeemDeviceAuthRequest() error = %v, want ErrDeviceAuthRequestNotFound (single-use)", err)
	}
}

func TestNewDeviceCode_And_NewUserCode_Unique(t *testing.T) {
	dc1, err := NewDeviceCode()
	if err != nil {
		t.Fatalf("NewDeviceCode() error = %v", err)
	}
	dc2, err := NewDeviceCode()
	if err != nil {
		t.Fatalf("NewDeviceCode() error = %v", err)
	}
	if dc1 == dc2 {
		t.Errorf("NewDeviceCode() returned duplicate values")
	}

	uc1, err := NewUserCode()
	if err != nil {
		t.Fatalf("NewUserCode() error = %v", err)
	}
	if len(uc1) != 9 || uc1[4] != '-' {
		t.Errorf("NewUserCode() = %q, want an 8-char code split XXXX-XXXX", uc1)
	}
}
