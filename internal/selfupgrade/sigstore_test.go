package selfupgrade

import (
	"bytes"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
)

const (
	testIssuer   = "https://token.actions.githubusercontent.com"
	testIdentity = "https://github.com/acme/repo/.github/workflows/release.yml@refs/tags/v1.0.0"
	testIDRegexp = `(?i)https://github.com/acme/repo/\.github/workflows/release\.yml@.*`
)

func TestVerifyEntity(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatalf("virtual sigstore: %v", err)
	}
	sums := []byte("abc  levelrail\n")
	tests := []struct {
		name     string
		identity string
		issuer   string
		artifact []byte
		wantErr  bool
	}{
		{name: "matching identity", identity: testIdentity, issuer: testIssuer, artifact: sums},
		{name: "wrong workflow", identity: "https://github.com/evil/repo/.github/workflows/release.yml@refs/heads/x", issuer: testIssuer, artifact: sums, wantErr: true},
		{name: "wrong issuer", identity: testIdentity, issuer: "https://accounts.example.com", artifact: sums, wantErr: true},
		{name: "tampered artifact", identity: testIdentity, issuer: testIssuer, artifact: []byte("def  levelrail\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signedIssuer := tt.issuer
			entity, err := vs.Sign(tt.identity, signedIssuer, sums)
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			err = verifyEntity(entity, vs, bytes.NewReader(tt.artifact), testIDRegexp, testIssuer)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
