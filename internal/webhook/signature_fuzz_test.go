package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func FuzzVerifySignature(f *testing.F) {
	secret := []byte("s3cret")
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	f.Add(good)
	f.Add("sha256=")
	f.Add("sha1=abcd")
	f.Add("sha256=zz")
	f.Add("")

	f.Fuzz(func(t *testing.T, header string) {
		if VerifySignature(secret, body, header) && header != good {
			t.Fatalf("header %q verified but is not the real signature", header)
		}
		if VerifySignature(nil, body, header) {
			t.Fatal("an empty secret must never verify")
		}
	})
}
