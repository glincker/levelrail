package cloudiac

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
)

func TestReleaseVerifier(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("virtual sigstore: %v", err)
	}
	archive := []byte("fixture archive bytes")
	sum := sha256.Sum256(archive)
	const name = "tofu_1.13.2_linux_amd64.tar.gz"
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n" + "00  other\n")
	path := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	goodID := tofuSigningWorkflow + "1.13"

	tests := []struct {
		name      string
		version   string
		identity  string
		issuer    string
		signed    []byte
		served    []byte
		archive   string
		noEntity  bool
		wantErr   error
		wantClean bool
	}{
		{name: "signed by the release workflow", version: "1.13.2", identity: goodID, issuer: tofuSigningIssuer, wantClean: true},
		{name: "signed from main", version: "1.13.2", identity: tofuSigningWorkflow[:len(tofuSigningWorkflow)-len("heads/v")] + "heads/main", issuer: tofuSigningIssuer, wantErr: ErrSignatureUnverified},
		{name: "other minor branch", version: "1.13.2", identity: tofuSigningWorkflow + "1.12", issuer: tofuSigningIssuer, wantErr: ErrSignatureUnverified},
		{name: "wrong issuer", version: "1.13.2", identity: goodID, issuer: "https://accounts.example.com", wantErr: ErrSignatureUnverified},
		{name: "checksum file swapped", version: "1.13.2", identity: goodID, issuer: tofuSigningIssuer, served: []byte(hex.EncodeToString(make([]byte, 32)) + "  " + name + "\n"), wantErr: ErrSignatureUnverified},
		{name: "archive not listed", version: "1.13.2", identity: goodID, issuer: tofuSigningIssuer, archive: "tofu_1.13.2_plan9_amd64.tar.gz", wantErr: ErrChecksumNotListed},
		{name: "pre-release refused", version: "1.14.0-beta1", identity: goodID, issuer: tofuSigningIssuer, wantErr: ErrSignatureUnverified},
		{name: "no bundle", version: "1.13.2", identity: goodID, issuer: tofuSigningIssuer, noEntity: true, wantErr: ErrSignatureUnverified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed := sums
			if tt.signed != nil {
				signed = tt.signed
			}
			entity, err := vs.Sign(tt.identity, tt.issuer, signed)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			served := signed
			if tt.served != nil {
				served = tt.served
			}
			src := func(context.Context, string) (ReleaseMaterial, error) {
				if tt.noEntity {
					return ReleaseMaterial{Sums: served}, nil
				}
				return ReleaseMaterial{Sums: served, Entity: entity}, nil
			}
			v := NewReleaseVerifier(tt.version, vs, src)
			v.ctThreshold = 0
			archiveName := name
			if tt.archive != "" {
				archiveName = tt.archive
			}
			err = v.Verify(context.Background(), archiveName, path)
			if tt.wantClean {
				if err != nil {
					t.Fatalf("verify: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("tampered archive", func(t *testing.T) {
		entity, _ := vs.Sign(goodID, tofuSigningIssuer, sums)
		v := NewReleaseVerifier("1.13.2", vs, func(context.Context, string) (ReleaseMaterial, error) {
			return ReleaseMaterial{Sums: sums, Entity: entity}, nil
		})
		v.ctThreshold = 0
		bad := filepath.Join(t.TempDir(), "bad")
		if err := os.WriteFile(bad, []byte("other bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := v.Verify(context.Background(), name, bad); !errors.Is(err, ErrChecksumMismatch) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unconfigured fails closed", func(t *testing.T) {
		if err := NewReleaseVerifier("1.13.2", nil, nil).Verify(context.Background(), name, path); !errors.Is(err, ErrSignatureUnverified) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestApplyLimitsAndFreeBytes(t *testing.T) {
	var cfg FetchConfig
	cfg.ApplyLimits(FetchLimits{MaxMB: 256, MinFreeMB: 4096, TimeoutSeconds: 300})
	if cfg.MaxBytes != 256<<20 || cfg.MinFreeBytes != 4096<<20 || cfg.Timeout.Seconds() != 300 || cfg.AllowChecksumOnly || cfg.FreeBytes == nil {
		t.Fatalf("cfg = %+v", cfg)
	}
	free, err := cfg.FreeBytes(t.TempDir())
	if err != nil && !errors.Is(err, ErrFreeSpaceUnsupported) {
		t.Fatalf("free bytes: %v", err)
	}
	if err == nil && free <= 0 {
		t.Fatalf("free = %d", free)
	}
}
