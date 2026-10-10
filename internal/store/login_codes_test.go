package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func newChallenge(id, user, browser string, now time.Time, ttl time.Duration) LoginCodeChallenge {
	return LoginCodeChallenge{ID: id, UserID: user, BrowserHash: browser, CodeHash: "h", Salt: "s",
		RequesterIP: "203.0.113.9", UserAgent: "ua", CreatedAt: now, ExpiresAt: now.Add(ttl)}
}

func TestLoginCodeChallengeLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	tests := []struct {
		name string
		run  func(t *testing.T, db *DB)
	}{
		{"live lookup is bound to the browser", func(t *testing.T, db *DB) {
			if _, err := db.GetLiveLoginCodeByBrowser(ctx, "other", now); !errors.Is(err, ErrLoginCodeNotFound) {
				t.Fatalf("other browser err = %v, want not found", err)
			}
			got, err := db.GetLiveLoginCodeByBrowser(ctx, "b1", now)
			if err != nil || got.ID != "c1" {
				t.Fatalf("got %+v, %v", got, err)
			}
		}},
		{"redeem is single use", func(t *testing.T, db *DB) {
			if ok, err := db.RedeemLoginCode(ctx, "c1", now); err != nil || !ok {
				t.Fatalf("first redeem = %v, %v", ok, err)
			}
			if ok, err := db.RedeemLoginCode(ctx, "c1", now); err != nil || ok {
				t.Fatalf("replay redeem = %v, %v, want false", ok, err)
			}
			if _, err := db.GetLiveLoginCodeByBrowser(ctx, "b1", now); !errors.Is(err, ErrLoginCodeNotFound) {
				t.Fatalf("redeemed challenge still live: %v", err)
			}
		}},
		{"expired challenge is not live and cannot be redeemed", func(t *testing.T, db *DB) {
			later := now.Add(11 * time.Minute)
			if _, err := db.GetLiveLoginCodeByBrowser(ctx, "b1", later); !errors.Is(err, ErrLoginCodeNotFound) {
				t.Fatalf("err = %v", err)
			}
			if ok, _ := db.RedeemLoginCode(ctx, "c1", later); ok {
				t.Fatal("expired challenge redeemed")
			}
			claimed, err := db.ClaimLapsedLoginCodes(ctx, later, 10)
			if err != nil || len(claimed) != 1 {
				t.Fatalf("claimed = %d, %v", len(claimed), err)
			}
			if again, _ := db.ClaimLapsedLoginCodes(ctx, later, 10); len(again) != 0 {
				t.Fatalf("expiry claimed twice: %d", len(again))
			}
		}},
		{"attempts are capped even under parallel guesses", func(t *testing.T, db *DB) {
			var wg sync.WaitGroup
			var mu sync.Mutex
			granted := 0
			for range 20 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, ok, err := db.ReserveLoginCodeAttempt(ctx, "c1", 5); err == nil && ok {
						mu.Lock()
						granted++
						mu.Unlock()
					}
				}()
			}
			wg.Wait()
			if granted != 5 {
				t.Fatalf("granted = %d, want 5", granted)
			}
			if ok, err := db.LockLoginCode(ctx, "c1", now); err != nil || !ok {
				t.Fatalf("lock = %v, %v", ok, err)
			}
			if ok, _ := db.RedeemLoginCode(ctx, "c1", now); ok {
				t.Fatal("locked challenge redeemed")
			}
		}},
		{"user listing excludes other users and unknown-account challenges", func(t *testing.T, db *DB) {
			if err := db.CreateLoginCodeChallenge(ctx, newChallenge("c2", "", "b2", now, 10*time.Minute)); err != nil {
				t.Fatal(err)
			}
			list, err := db.ListLiveLoginCodesForUser(ctx, "u1", now)
			if err != nil || len(list) != 1 || list[0].ID != "c1" {
				t.Fatalf("list = %+v, %v", list, err)
			}
			if empty, _ := db.ListLiveLoginCodesForUser(ctx, "", now); len(empty) != 0 {
				t.Fatalf("empty user listed %d", len(empty))
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			if err := db.CreateLoginCodeChallenge(ctx, newChallenge("c1", "u1", "b1", now, 10*time.Minute)); err != nil {
				t.Fatal(err)
			}
			tt.run(t, db)
		})
	}
}

func TestLoginApprovalLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	tests := []struct {
		name        string
		decider     string
		approve     bool
		at          time.Time
		wantDecided bool
		wantConsume bool
	}{
		{"owner approves", "u1", true, now, true, true},
		{"owner denies", "u1", false, now, true, false},
		{"another user cannot decide", "u2", true, now, false, false},
		{"expired cannot be decided", "u1", true, now.Add(time.Hour), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			a := LoginApproval{ID: "a1", UserID: "u1", BrowserHash: "b1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
			if err := db.CreateLoginApproval(ctx, a); err != nil {
				t.Fatal(err)
			}
			ok, err := db.DecideLoginApproval(ctx, "a1", tt.decider, tt.approve, "session:"+tt.decider, tt.at)
			if err != nil || ok != tt.wantDecided {
				t.Fatalf("decide = %v, %v, want %v", ok, err, tt.wantDecided)
			}
			consumed, err := db.ConsumeLoginApproval(ctx, "a1", now)
			if err != nil || consumed != tt.wantConsume {
				t.Fatalf("consume = %v, %v, want %v", consumed, err, tt.wantConsume)
			}
			if again, _ := db.ConsumeLoginApproval(ctx, "a1", now); again {
				t.Fatal("approval consumed twice")
			}
		})
	}
}

func TestTrustedDeviceRevoke(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	d := TrustedDevice{ID: "td1", UserID: "u1", TokenHash: "th", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := db.CreateTrustedDevice(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTrustedDeviceByHash(ctx, "th", now); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if ok, _ := db.RevokeTrustedDevice(ctx, "u2", "td1", now); ok {
		t.Fatal("another user revoked the device")
	}
	if ok, err := db.RevokeTrustedDevice(ctx, "u1", "td1", now); err != nil || !ok {
		t.Fatalf("revoke = %v, %v", ok, err)
	}
	if _, err := db.GetTrustedDeviceByHash(ctx, "th", now); !errors.Is(err, ErrTrustedDeviceNotFound) {
		t.Fatalf("revoked device still trusted: %v", err)
	}
	if list, _ := db.ListTrustedDevices(ctx, "u1", now); len(list) != 0 {
		t.Fatalf("revoked device listed")
	}
}

func TestSupersedeLoginApprovals_OnlyOwnPending(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Now()
	for _, a := range []LoginApproval{
		{ID: "a1", UserID: "u1", BrowserHash: "b1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "a2", UserID: "u1", BrowserHash: "b2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "a3", UserID: "u2", BrowserHash: "b3", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := db.CreateLoginApproval(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := db.SupersedeLoginApprovals(ctx, "u1", now)
	if err != nil || len(moved) != 2 {
		t.Fatalf("superseded = %d, %v, want 2", len(moved), err)
	}
	for _, tc := range []struct{ user, want string }{{"u1", ""}, {"u2", "a3"}} {
		list, _ := db.ListPendingLoginApprovalsForUser(ctx, tc.user, now)
		got := ""
		if len(list) > 0 {
			got = list[0].ID
		}
		if got != tc.want {
			t.Fatalf("pending for %s = %q, want %q", tc.user, got, tc.want)
		}
	}
}

func TestTrustedDeviceBulkRevokeExtendPrune(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	seed := func(t *testing.T, db *DB) {
		for _, d := range []TrustedDevice{
			{ID: "td1", UserID: "u1", TokenHash: "h1", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)},
			{ID: "td2", UserID: "u1", TokenHash: "h2", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour)},
			{ID: "td3", UserID: "u2", TokenHash: "h3", CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(-time.Minute)},
		} {
			if err := db.CreateTrustedDevice(ctx, d); err != nil {
				t.Fatal(err)
			}
		}
	}
	tests := []struct {
		name string
		run  func(t *testing.T, db *DB)
	}{
		{"revoke all touches one user only", func(t *testing.T, db *DB) {
			if n, err := db.RevokeAllTrustedDevices(ctx, "u1", now); err != nil || n != 2 {
				t.Fatalf("revoked = %d, %v", n, err)
			}
			if _, err := db.GetTrustedDeviceByHash(ctx, "h1", now); !errors.Is(err, ErrTrustedDeviceNotFound) {
				t.Fatalf("u1 device still trusted: %v", err)
			}
		}},
		{"extend pushes expiry", func(t *testing.T, db *DB) {
			if err := db.ExtendTrustedDevice(ctx, "td1", now.Add(48*time.Hour), now); err != nil {
				t.Fatal(err)
			}
			if _, err := db.GetTrustedDeviceByHash(ctx, "h1", now.Add(24*time.Hour)); err != nil {
				t.Fatalf("extended device not live tomorrow: %v", err)
			}
		}},
		{"prune deletes expired rows only", func(t *testing.T, db *DB) {
			if err := db.PruneTrustedDevices(ctx, now); err != nil {
				t.Fatal(err)
			}
			var n int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM trusted_devices`).Scan(&n); err != nil || n != 2 {
				t.Fatalf("rows = %d, %v, want 2", n, err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			seed(t, db)
			tt.run(t, db)
		})
	}
}

func TestCodeLoginSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	if _, ok, err := db.GetCodeLoginSettings(ctx); err != nil || ok {
		t.Fatalf("fresh db: ok = %v, err = %v", ok, err)
	}
	want := CodeLoginSettings{Admins: true, Others: false, UpdatedAt: time.Now()}
	if err := db.SaveCodeLoginSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetCodeLoginSettings(ctx)
	if err != nil || !ok || got.Admins != want.Admins || got.Others != want.Others {
		t.Fatalf("got %+v ok=%v err=%v", got, ok, err)
	}
}
