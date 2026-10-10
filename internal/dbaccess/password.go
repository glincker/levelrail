package dbaccess

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
)

const (
	passwordBytes  = 24
	scramIter      = 4096
	scramSaltBytes = 16
)

// GeneratePassword returns a random URL-safe password.
func GeneratePassword() (string, error) {
	buf := make([]byte, passwordBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("dbaccess: generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ScramVerifier hashes password into a Postgres SCRAM-SHA-256 verifier, so
// the SQL that sets it never carries the plaintext (a failed statement is
// logged verbatim by the server).
func ScramVerifier(password string) (string, error) {
	salt := make([]byte, scramSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("dbaccess: scram salt: %w", err)
	}
	return scramVerifierWithSalt(password, salt)
}

func scramVerifierWithSalt(password string, salt []byte) (string, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, scramIter, sha256.Size)
	if err != nil {
		return "", fmt.Errorf("dbaccess: scram key derivation: %w", err)
	}
	clientKey := hmacSHA256(salted, "Client Key")
	stored := sha256.Sum256(clientKey)
	serverKey := hmacSHA256(salted, "Server Key")
	enc := base64.StdEncoding
	return "SCRAM-SHA-256$" + strconv.Itoa(scramIter) + ":" + enc.EncodeToString(salt) +
		"$" + enc.EncodeToString(stored[:]) + ":" + enc.EncodeToString(serverKey), nil
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}
