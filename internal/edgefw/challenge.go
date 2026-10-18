package edgefw

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math/bits"
	"net/netip"
	"strconv"
	"sync/atomic"
	"time"
)

// ChallengePath is the reserved path the challenge page posts its proof to.
const ChallengePath = "/.well-known/lr-challenge"

// PassCookie is the name of the short-lived cookie a solved challenge earns.
const PassCookie = "__lr_pass"

const (
	kindToken byte = 'c'
	kindPass  byte = 'p'
)

// Signer issues and checks the challenge token and the pass cookie. Both are
// HMAC-SHA256 signed, bound to the client's network prefix and short lived.
type Signer struct {
	key atomic.Pointer[[]byte]
}

// NewSigner starts with a random per-process key; SetKey replaces it with
// one derived from the master key so cookies survive a restart.
func NewSigner() *Signer {
	s := &Signer{}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	s.key.Store(&k)
	return s
}

// SetKey installs key; a nil or short key is ignored.
func (s *Signer) SetKey(key []byte) {
	if len(key) < 16 {
		return
	}
	k := append([]byte(nil), key...)
	s.key.Store(&k)
}

// prefixBytes is the part of ip a token binds to: /24 for IPv4, /64 for IPv6.
func prefixBytes(ip netip.Addr) []byte {
	if !ip.IsValid() {
		return nil
	}
	if ip.Is4() {
		b := ip.As4()
		return b[:3]
	}
	b := ip.As16()
	return b[:8]
}

func (s *Signer) mac(msg []byte) []byte {
	h := hmac.New(sha256.New, *s.key.Load())
	h.Write(msg)
	return h.Sum(nil)[:16]
}

func (s *Signer) seal(kind byte, exp time.Time, bitsReq int, nonce []byte, ip netip.Addr) string {
	body := make([]byte, 0, 40)
	body = append(body, kind)
	body = binary.BigEndian.AppendUint64(body, uint64(exp.Unix())) //nolint:gosec // expiry is a positive unix time
	body = append(body, byte(bitsReq))                             //nolint:gosec // difficulty is capped at 28
	body = append(body, nonce...)
	enc := base64.RawURLEncoding
	msg := append(append([]byte(nil), body...), prefixBytes(ip)...)
	return enc.EncodeToString(body) + "." + enc.EncodeToString(s.mac(msg))
}

func (s *Signer) open(kind byte, tok string, ip netip.Addr, now time.Time) (bitsReq int, ok bool) {
	enc := base64.RawURLEncoding
	dot := -1
	for i := 0; i < len(tok); i++ {
		if tok[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 || len(tok) > 256 {
		return 0, false
	}
	body, err := enc.DecodeString(tok[:dot])
	if err != nil || len(body) < 10 || body[0] != kind {
		return 0, false
	}
	sig, err := enc.DecodeString(tok[dot+1:])
	if err != nil {
		return 0, false
	}
	msg := append(append([]byte(nil), body...), prefixBytes(ip)...)
	if !hmac.Equal(sig, s.mac(msg)) {
		return 0, false
	}
	if !now.Before(time.Unix(int64(binary.BigEndian.Uint64(body[1:9])), 0)) { //nolint:gosec // written by seal as a positive unix time
		return 0, false
	}
	return int(body[9]), true
}

// NewChallenge returns a signed token for ip that the page must solve with
// the given difficulty.
func (s *Signer) NewChallenge(ip netip.Addr, difficulty int, ttl time.Duration, now time.Time) string {
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	return s.seal(kindToken, now.Add(ttl), difficulty, nonce, ip)
}

// VerifyProof checks the token's signature, expiry and binding, then that
// sha256(token + ":" + counter) has the required leading zero bits.
func (s *Signer) VerifyProof(token, counter string, ip netip.Addr, now time.Time) bool {
	need, ok := s.open(kindToken, token, ip, now)
	if !ok || counter == "" || len(counter) > 12 {
		return false
	}
	if _, err := strconv.ParseUint(counter, 10, 32); err != nil {
		return false
	}
	return LeadingZeroBits(sha256.Sum256([]byte(token+":"+counter))) >= need
}

// NewPass returns a pass cookie value for ip valid for ttl.
func (s *Signer) NewPass(ip netip.Addr, ttl time.Duration, now time.Time) string {
	return s.seal(kindPass, now.Add(ttl), 0, nil, ip)
}

// PassValid reports whether cookie is an unexpired pass for ip's network.
func (s *Signer) PassValid(cookie string, ip netip.Addr, now time.Time) bool {
	if cookie == "" {
		return false
	}
	_, ok := s.open(kindPass, cookie, ip, now)
	return ok
}

// LeadingZeroBits counts the zero bits at the start of sum.
func LeadingZeroBits(sum [32]byte) int {
	n := 0
	for _, b := range sum {
		if b == 0 {
			n += 8
			continue
		}
		return n + bits.LeadingZeros8(b)
	}
	return n
}
