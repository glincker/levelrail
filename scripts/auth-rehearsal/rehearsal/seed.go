package rehearsal

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// StoreFile is the database name the control plane uses in its data dir.
	StoreFile = "levelrail.db"
	// MasterKeyFile is the master key name the control plane uses in its data dir.
	MasterKeyFile = "master.key"
	// TOTPSecretKey is the env-key name the control plane stores TOTP secrets under.
	TOTPSecretKey = "secret"

	ghostOwners = 20
)

var abilitySets = []struct {
	weight int
	set    []string
}{
	{5, []string{"root"}},
	{15, []string{"read", "read:sensitive", "write", "write:sensitive", "deploy"}},
	{20, []string{"read", "write", "deploy"}},
	{25, []string{"read", "read:sensitive"}},
	{35, []string{"read"}},
}

type seeder struct {
	cfg Config
	rng *rand.Rand
	db  *store.DB
	mgr *secrets.Manager
	m   *Manifest
	now time.Time
}

// Seed builds a synthetic legacy database in cfg.Dir through the store layer
// and returns the answer key. The same Config always yields the same data.
func Seed(ctx context.Context, cfg Config) (*Manifest, error) {
	if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("rehearsal: create dir: %w", err)
	}
	db, err := store.Open(ctx, filepath.Join(cfg.Dir, StoreFile))
	if err != nil {
		return nil, fmt.Errorf("rehearsal: open store: %w", err)
	}
	defer func() { _ = db.Close() }()
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		return nil, fmt.Errorf("rehearsal: master key: %w", err)
	}
	if err := secrets.PersistMasterKeyFile(filepath.Join(cfg.Dir, MasterKeyFile), mk.String()); err != nil {
		return nil, fmt.Errorf("rehearsal: persist master key: %w", err)
	}
	s := &seeder{
		cfg: cfg, db: db, mgr: secrets.NewManager(db, mk), m: &Manifest{Config: cfg},
		rng: rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)), //nolint:gosec // deterministic fixture data
		now: time.Now().UTC().Truncate(time.Millisecond),
	}
	if err := s.users(ctx); err != nil {
		return nil, err
	}
	if err := s.tokens(ctx); err != nil {
		return nil, err
	}
	if err := s.passkeys(ctx); err != nil {
		return nil, err
	}
	if err := s.totp(ctx); err != nil {
		return nil, err
	}
	if err := s.oauth(ctx); err != nil {
		return nil, err
	}
	s.tally()
	return s.m, s.m.Save()
}

func (s *seeder) pickAbilities() []string {
	n := s.rng.IntN(100)
	for _, a := range abilitySets {
		if n < a.weight {
			return append([]string(nil), a.set...)
		}
		n -= a.weight
	}
	return []string{"read"}
}

func (s *seeder) randBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(s.rng.UintN(256)) //nolint:gosec // bounded to a byte
	}
	return b
}

func (s *seeder) specialUser(i int) (email, name, kind string) {
	switch i {
	case 0:
		return "admin", "Legacy Admin", KindNoAt
	case 1:
		return strings.Repeat("a", 64) + "@" + strings.Repeat("sub.", 45) + "example.test", "Long Email", KindLong
	case 2:
		return strings.Repeat("x", 300) + "@example.test", strings.Repeat("N", 600), KindLong
	case 3:
		return "jürgen.müller+日本語@exämple.test", "Jürgen Müller 日本語 \U0001F600", KindUnicode
	case 4:
		return "Ünï.CafÉ@Exämple.test", "Ünï", KindUnicode
	case 5:
		return "Mixed.Case@Example.TEST", "Mixed Case", KindMixedCase
	}
	if i%20 == 0 {
		return fmt.Sprintf("Person.%05d@Example.test", i), fmt.Sprintf("Person %d", i), KindMixedCase
	}
	return fmt.Sprintf("user%05d@example.test", i), fmt.Sprintf("User %d", i), KindNormal
}

func (s *seeder) users(ctx context.Context) error {
	total := s.cfg.Users
	pws := make([]string, total)
	for i := range pws {
		if _, _, kind := s.specialUser(i); kind != KindNormal || i%17 != 0 {
			pws[i] = fmt.Sprintf("pw-%d-%d-%08x ✓", s.cfg.Seed, i, s.rng.Uint32())
		}
	}
	hashes, err := hashAll(pws, s.cfg.BcryptCost)
	if err != nil {
		return err
	}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < total; i++ {
		email, name, kind := s.specialUser(i)
		u := User{ID: fmt.Sprintf("user_%08x", i), Email: email, Name: name, Kind: kind, Abilities: s.pickAbilities()}
		if i == 0 {
			u.ID, u.Abilities = "user_legacy_admin", []string{"root"}
		}
		su := store.User{
			ID: u.ID, Email: u.Email, DisplayName: u.Name, Abilities: u.Abilities,
			IsFirstUser: i == 0, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if pws[i] != "" {
			h := hashes[i]
			su.PasswordHash, u.Password = &h, pws[i]
		} else if kind == KindNormal {
			u.Kind = KindOAuthOnly
		}
		if i%3 == 0 {
			t := base.Add(time.Duration(i)*time.Minute + time.Hour)
			su.LastLoginAt = &t
		}
		if err := s.db.CreateUser(ctx, su); err != nil {
			return fmt.Errorf("rehearsal: create user %s: %w", u.ID, err)
		}
		s.m.Users = append(s.m.Users, u)
	}
	return s.caseDupes(ctx, base)
}

func (s *seeder) caseDupes(ctx context.Context, base time.Time) error {
	for i := 0; i < s.cfg.CaseDupes && i < len(s.m.Users); i++ {
		orig := s.m.Users[10+i]
		d := User{
			ID: fmt.Sprintf("user_dupe%04x", i), Email: strings.ToUpper(orig.Email), Name: "Dupe of " + orig.Name,
			Kind: KindCaseDupe, DupeOf: orig.ID, Abilities: []string{"read"},
		}
		if err := s.db.CreateUser(ctx, store.User{
			ID: d.ID, Email: d.Email, DisplayName: d.Name, Abilities: d.Abilities,
			CreatedAt: base.Add(time.Duration(s.cfg.Users+i) * time.Minute),
		}); err != nil {
			return fmt.Errorf("rehearsal: create case duplicate %s: %w", d.ID, err)
		}
		s.m.Users = append(s.m.Users, d)
	}
	return nil
}

func hashAll(pws []string, cost int) ([]string, error) {
	out := make([]string, len(pws))
	errs := make(chan error, len(pws))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, p := range pws {
		if p == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h, err := bcrypt.GenerateFromPassword([]byte(p), cost)
			if err != nil {
				errs <- fmt.Errorf("rehearsal: bcrypt: %w", err)
				return
			}
			out[i] = string(h)
		}()
	}
	wg.Wait()
	close(errs)
	if err := <-errs; err != nil {
		return nil, err
	}
	return out, nil
}

func (s *seeder) tokens(ctx context.Context) error {
	owners := make([]User, 0, len(s.m.Users)+ghostOwners)
	for _, u := range s.m.Users {
		if u.Kind != KindCaseDupe {
			owners = append(owners, u)
		}
	}
	ghosts := make([]User, ghostOwners)
	for i := range ghosts {
		ghosts[i] = User{ID: fmt.Sprintf("user_ghost%04x", i), Email: fmt.Sprintf("ghost%d@example.test", i), Name: "Ghost", Abilities: []string{"read", "write"}}
		if err := s.db.CreateUser(ctx, store.User{ID: ghosts[i].ID, Email: ghosts[i].Email, DisplayName: "Ghost", Abilities: ghosts[i].Abilities, CreatedAt: s.now}); err != nil {
			return fmt.Errorf("rehearsal: create ghost owner: %w", err)
		}
	}
	for i := 0; i < s.cfg.Tokens; i++ {
		raw := base64.RawURLEncoding.EncodeToString(s.randBytes(32))
		sum := sha256.Sum256([]byte(raw))
		t := Token{ID: fmt.Sprintf("tok_%08x", i), Raw: raw, State: StateActive}
		st := store.APIToken{ID: t.ID, Name: fmt.Sprintf("token %d ✓", i), TokenHash: hex.EncodeToString(sum[:]), CreatedAt: s.now.Add(-time.Duration(s.cfg.Tokens-i) * time.Minute)}
		owner := owners[s.rng.IntN(len(owners))]
		t.Abilities = s.tokenAbilities(owner)
		switch n := i % 100; {
		case n < 5:
			t.State = StateExpired
			e := s.now.Add(-24 * time.Hour)
			st.ExpiresAt = &e
		case n < 10:
			t.State = StateRevoked
		case n < 12:
			t.State = StateOrphan
			owner = ghosts[s.rng.IntN(len(ghosts))]
			t.Abilities = []string{"read"}
		case n == 12 && i < 1200 || i == 12:
			t.State, t.Abilities = StateOwnerless, []string{"read"}
			owner = User{}
			st.Name = "AI Assistant (internal)"
		case n < 20:
			e := s.now.Add(30 * 24 * time.Hour)
			st.ExpiresAt = &e
		}
		t.Owner, st.OwnerUserID, st.Abilities = owner.ID, owner.ID, t.Abilities
		if err := s.db.SaveAPIToken(ctx, st); err != nil {
			return fmt.Errorf("rehearsal: save token %s: %w", t.ID, err)
		}
		if t.State == StateRevoked {
			if err := s.db.RevokeAPIToken(ctx, t.ID); err != nil {
				return fmt.Errorf("rehearsal: revoke token %s: %w", t.ID, err)
			}
		}
		s.m.Tokens = append(s.m.Tokens, t)
	}
	for _, g := range ghosts {
		if err := s.db.DeleteUser(ctx, g.ID); err != nil {
			return fmt.Errorf("rehearsal: delete ghost owner: %w", err)
		}
	}
	return nil
}

func (s *seeder) tokenAbilities(owner User) []string {
	if s.rng.IntN(50) == 0 && !contains(owner.Abilities, "root") {
		return []string{"write", "deploy"}
	}
	if contains(owner.Abilities, "root") {
		if s.rng.IntN(2) == 0 {
			return []string{"root"}
		}
		return []string{"read", "deploy"}
	}
	var out []string
	for _, a := range owner.Abilities {
		if s.rng.IntN(10) < 6 {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		out = []string{owner.Abilities[0]}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (s *seeder) passkeys(ctx context.Context) error {
	for i := 0; i < s.cfg.Passkeys; i++ {
		u := &s.m.Users[s.rng.IntN(s.cfg.Users)]
		credID, pub := s.randBytes(32), s.randBytes(77)
		sign := uint32(s.rng.IntN(1000)) //nolint:gosec // bounded fixture value
		err := s.db.SavePasskeyCredential(ctx, store.PasskeyCredential{
			ID: fmt.Sprintf("pk_%06d", i), UserID: u.ID, CredentialID: base64.RawURLEncoding.EncodeToString(credID),
			PublicKey: pub, SignCount: sign, AAGUID: base64.RawURLEncoding.EncodeToString(s.randBytes(16)),
			Transports: []string{"usb", "internal"}, Label: fmt.Sprintf("key %d", i), BackupEligible: i%2 == 0,
			CreatedAt: s.now.Add(-time.Duration(i) * time.Minute),
		})
		if err != nil {
			return fmt.Errorf("rehearsal: save passkey %d: %w", i, err)
		}
		u.Passkeys++
		s.m.Passkeys = append(s.m.Passkeys, Passkey{UserID: u.ID, CredentialID: credID, PublicKey: pub, SignCount: sign})
	}
	return nil
}

func (s *seeder) totp(ctx context.Context) error {
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	done := 0
	for i := range s.m.Users {
		if done >= s.cfg.TOTPUsers {
			break
		}
		u := &s.m.Users[i]
		if u.Kind == KindCaseDupe || (u.Password == "" && i > 10) {
			continue
		}
		u.TOTPSecret = enc.EncodeToString(s.randBytes(20))
		if err := s.mgr.SetValue(ctx, store.UserTOTPSecretsKey(u.ID), TOTPSecretKey, u.TOTPSecret); err != nil {
			return fmt.Errorf("rehearsal: store totp secret for %s: %w", u.ID, err)
		}
		if err := s.db.EnableUserTOTP(ctx, u.ID, s.now); err != nil {
			return fmt.Errorf("rehearsal: enable totp for %s: %w", u.ID, err)
		}
		codes := make([]string, 10)
		for j := range codes {
			sum := sha256.Sum256([]byte(enc.EncodeToString(s.randBytes(10))))
			codes[j] = hex.EncodeToString(sum[:])
		}
		if err := s.db.ReplaceUserRecoveryCodes(ctx, u.ID, codes); err != nil {
			return fmt.Errorf("rehearsal: store recovery codes for %s: %w", u.ID, err)
		}
		u.RecoveryCodes = len(codes)
		done++
	}
	return nil
}

func (s *seeder) oauth(ctx context.Context) error {
	providers := []string{store.OAuthProviderGoogle, store.OAuthProviderGitHub, store.OAuthProviderOIDC, store.OAuthProviderMicrosoft}
	for i := 0; i < s.cfg.OAuth && i < s.cfg.Users; i++ {
		u := s.m.Users[(i*7)%s.cfg.Users]
		p := providers[i%len(providers)]
		id := OAuthIdentity{UserID: u.ID, Provider: p, ProviderUserID: fmt.Sprintf("%s-sub-%06d", p, i)}
		err := s.db.SaveOAuthIdentity(ctx, store.OAuthIdentity{
			ID: fmt.Sprintf("oid_%06d", i), UserID: id.UserID, Provider: p, ProviderUserID: id.ProviderUserID, CreatedAt: s.now,
		})
		if errors.Is(err, store.ErrOAuthIdentityAlreadyLinked) {
			continue
		}
		if err != nil {
			return fmt.Errorf("rehearsal: save oauth identity %d: %w", i, err)
		}
		s.m.OAuth = append(s.m.OAuth, id)
	}
	return nil
}

func (s *seeder) tally() {
	e := Expected{TokensByState: map[string]int{}, UsersByKind: map[string]int{}}
	for _, u := range s.m.Users {
		e.Users++
		e.UsersByKind[u.Kind]++
		if u.Password != "" {
			e.Passwords++
		}
		if u.Kind == KindCaseDupe {
			e.CaseCollisions++
		}
		if u.TOTPSecret != "" {
			e.TOTP++
			e.ExpectedRecovered++
		}
	}
	for _, t := range s.m.Tokens {
		e.TokensByState[t.State]++
		if t.State == StateOrphan || t.State == StateOwnerless {
			e.TokensSkipped++
		} else {
			e.Tokens++
		}
	}
	e.Passkeys = len(s.m.Passkeys)
	e.OAuth = len(s.m.OAuth)
	s.m.Expected = e
}
