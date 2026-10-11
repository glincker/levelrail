package selfupgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "levelrail-linux-amd64")
	if err := os.WriteFile(bin, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	const helloSum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	tests := []struct {
		name string
		sums string
		want error
	}{
		{name: "match", sums: helloSum + "  levelrail-linux-amd64\n"},
		{name: "binary marker", sums: helloSum + " *levelrail-linux-amd64\n"},
		{name: "mismatch", sums: strings.Repeat("a", 64) + "  levelrail-linux-amd64\n", want: ErrChecksumMismatch},
		{name: "not listed", sums: helloSum + "  other\n", want: ErrChecksumMissing},
		{name: "garbage", sums: "not a checksum file\n", want: ErrChecksumMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sums := filepath.Join(dir, "checksums.txt")
			if err := os.WriteFile(sums, []byte(tt.sums), 0o600); err != nil {
				t.Fatal(err)
			}
			err := VerifyChecksum(bin, sums, "levelrail-linux-amd64")
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSignerModes(t *testing.T) {
	ok := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	bad := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("no matching signatures"), errors.New("exit 1")
	}
	tests := []struct {
		name   string
		s      Signer
		bundle string
		want   error
	}{
		{name: "verified", s: Signer{Mode: VerifyAuto, Run: ok}, bundle: "b"},
		{name: "invalid always fails", s: Signer{Mode: VerifyAuto, Run: bad}, bundle: "b", want: ErrSignatureInvalid},
		{name: "off skips", s: Signer{Mode: VerifyOff, Run: bad}, bundle: "b"},
		{name: "auto without bundle", s: Signer{Mode: VerifyAuto, Run: ok}},
		{name: "require without bundle", s: Signer{Mode: VerifyRequire, Run: ok}, want: ErrSignatureRequired},
		{name: "require without cosign", s: Signer{Mode: VerifyRequire, CosignPath: "/nonexistent/cosign"}, bundle: "b", want: ErrSignatureRequired},
		{name: "auto without cosign", s: Signer{Mode: VerifyAuto, CosignPath: "/nonexistent/cosign"}, bundle: "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.s.Verify(context.Background(), "sums", tt.bundle)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
