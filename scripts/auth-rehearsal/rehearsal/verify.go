package rehearsal

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

const wrongPassword = "definitely-not-the-password-1"

// SampleUsers picks users for sign-in parity: every unusual kind first, then
// TOTP users, then ordinary password users, up to n.
func SampleUsers(m *Manifest, n int) []User {
	var out []User
	taken := map[string]bool{}
	add := func(u User) {
		if !taken[u.ID] && len(out) < n {
			taken[u.ID] = true
			out = append(out, u)
		}
	}
	oauthOnly := 0
	for _, u := range m.Users {
		switch {
		case u.Kind == KindCaseDupe:
		case u.Kind == KindOAuthOnly:
			if oauthOnly < 15 {
				oauthOnly++
				add(u)
			}
		case u.Kind != KindNormal:
			add(u)
		}
	}
	totp := 0
	for _, u := range m.Users {
		if u.TOTPSecret != "" && u.Kind != KindCaseDupe && totp < 40 {
			totp++
			add(u)
		}
	}
	for i := 0; i < len(m.Users) && len(out) < n; i++ {
		u := m.Users[(i*13)%len(m.Users)]
		if u.Kind == KindNormal {
			add(u)
		}
	}
	return out
}

// SampleTokens picks tokens for authentication parity: a bounded slice of every
// non-active state, then active tokens, up to n.
func SampleTokens(m *Manifest, n int) []Token {
	var out []Token
	perState := map[string]int{}
	for _, t := range m.Tokens {
		if t.State != StateActive && perState[t.State] < n/8 {
			perState[t.State]++
			out = append(out, t)
		}
	}
	for i := 0; i < len(m.Tokens) && len(out) < n; i++ {
		if t := m.Tokens[(i*7)%len(m.Tokens)]; t.State == StateActive && !slices.ContainsFunc(out, func(o Token) bool { return o.ID == t.ID }) {
			out = append(out, t)
		}
	}
	return out
}

func (e *Env) passwordHash(ctx context.Context, legacyID string) (string, bool, error) {
	var h string
	err := e.DB.QueryRowContext(ctx, `SELECT password_hash FROM theauth_user_passwords WHERE user_id = ?`, e.Mapping[legacyID]).Scan(&h)
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("rehearsal: read password hash: %w", err)
	}
	return h, true, nil
}

// CheckUser signs one seeded user in through the library path and returns what went wrong.
func (e *Env) CheckUser(ctx context.Context, u User) []string {
	tag := fmt.Sprintf("user %s (%s)", u.ID, u.Kind)
	if u.Password == "" {
		r, err := e.post(ctx, "/email-password/signin", map[string]string{"email": u.Email, "password": wrongPassword}, nil)
		if err != nil || r.Status == http.StatusOK {
			return []string{fmt.Sprintf("%s: password-less user signed in (status %d, err %v)", tag, r.Status, err)}
		}
		if _, ok, _ := e.passwordHash(ctx, u.ID); ok {
			return []string{tag + ": password-less user gained a library password row"}
		}
		return nil
	}
	var bad []string
	before, ok, err := e.passwordHash(ctx, u.ID)
	if err != nil || !ok || !strings.HasPrefix(before, "$2") {
		return []string{fmt.Sprintf("%s: expected a copied bcrypt hash, got ok=%v prefix=%.6q err=%v", tag, ok, before, err)}
	}
	if r, err := e.post(ctx, "/email-password/signin", map[string]string{"email": u.Email, "password": wrongPassword}, nil); err != nil || r.Status == http.StatusOK {
		bad = append(bad, tag+": wrong password signed in")
	}
	r, err := e.post(ctx, "/email-password/signin", map[string]string{"email": u.Email, "password": u.Password}, nil)
	if err != nil || r.Status != http.StatusOK {
		return append(bad, fmt.Sprintf("%s: first signin status %d body %.120q err %v", tag, r.Status, r.Body, err))
	}
	if u.TOTPSecret != "" {
		bad = append(bad, e.checkTOTP(ctx, u, tag, r)...)
	}
	after, _, _ := e.passwordHash(ctx, u.ID)
	if !strings.HasPrefix(after, "$argon2id$") {
		return append(bad, fmt.Sprintf("%s: hash not upgraded to argon2id after first signin (prefix %.8q)", tag, after))
	}
	r2, err := e.post(ctx, "/email-password/signin", map[string]string{"email": u.Email, "password": u.Password}, nil)
	if err != nil || r2.Status != http.StatusOK {
		return append(bad, fmt.Sprintf("%s: second signin status %d err %v", tag, r2.Status, err))
	}
	if again, _, _ := e.passwordHash(ctx, u.ID); again != after {
		bad = append(bad, tag+": hash changed on second signin, expected the argon2id hash to be used as is")
	}
	return bad
}

func (e *Env) checkTOTP(ctx context.Context, u User, tag string, signin reply) []string {
	if !strings.Contains(signin.Body, "totp_required") {
		return []string{fmt.Sprintf("%s: totp user signin did not ask for a code: %.120q", tag, signin.Body)}
	}
	code, err := TOTPCode(u.TOTPSecret, time.Now())
	if err != nil {
		return []string{tag + ": " + err.Error()}
	}
	r, err := e.post(ctx, "/totp/verify", map[string]string{"code": code}, signin.Cookies)
	if err != nil || r.Status != http.StatusOK {
		return []string{fmt.Sprintf("%s: seeded totp code rejected (status %d body %.120q err %v)", tag, r.Status, r.Body, err)}
	}
	return nil
}

// CheckUsers runs CheckUser over users with a small worker pool.
func (e *Env) CheckUsers(ctx context.Context, users []User, workers int) []string {
	var (
		mu  sync.Mutex
		bad []string
		wg  sync.WaitGroup
	)
	ch := make(chan User)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range ch {
				if f := e.CheckUser(ctx, u); len(f) > 0 {
					mu.Lock()
					bad = append(bad, f...)
					mu.Unlock()
				}
			}
		}()
	}
	for _, u := range users {
		ch <- u
	}
	close(ch)
	wg.Wait()
	return bad
}

func clamp(tokenAbilities, owner []string) ([]string, error) {
	req, err := authengine.MapAbilities(tokenAbilities)
	if err != nil {
		return nil, fmt.Errorf("map abilities: %w", err)
	}
	held, err := authengine.MapAbilities(owner)
	if err != nil {
		return nil, fmt.Errorf("map owner abilities: %w", err)
	}
	var out []string
	for _, a := range req {
		switch {
		case a == theauth.AbilityRoot:
			if slices.Contains(held, theauth.AbilityRoot) {
				out = append(out, a)
			}
		case slices.Contains(held, theauth.AbilityRoot) || slices.Contains(held, a):
			out = append(out, a)
		}
	}
	return out, nil
}

// CheckTokens authenticates sampled legacy tokens through the library and
// returns every divergence from what the legacy state says should happen.
func (e *Env) CheckTokens(ctx context.Context, m *Manifest, sample []Token) []string {
	owners := map[string][]string{}
	for _, u := range m.Users {
		owners[u.ID] = u.Abilities
	}
	var bad []string
	for i, t := range sample {
		tag := fmt.Sprintf("token %s (%s)", t.ID, t.State)
		p, err := e.Eng.Auth().AuthenticateAPIToken(ctx, t.Raw)
		if t.State != StateActive {
			if err == nil {
				bad = append(bad, tag+": authenticated but must be rejected")
			}
			continue
		}
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: rejected: %v", tag, err))
			continue
		}
		if want := e.Mapping[t.Owner]; p.UserID.String() != want {
			bad = append(bad, fmt.Sprintf("%s: owner %s, want %s", tag, p.UserID, want))
		}
		want, err := clamp(t.Abilities, owners[t.Owner])
		if err != nil {
			bad = append(bad, tag+": "+err.Error())
			continue
		}
		got := slices.Clone(p.Abilities)
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			bad = append(bad, fmt.Sprintf("%s: abilities %v, want %v (legacy %v)", tag, got, want, t.Abilities))
		}
		if i%20 == 0 {
			if st, err := e.bearerStatus(ctx, t.Raw); err != nil || st != http.StatusOK {
				bad = append(bad, fmt.Sprintf("%s: /tokens/current status %d err %v", tag, st, err))
			}
		}
	}
	return bad
}

// CheckPasskeys reads every seeded passkey back through the library storage.
func (e *Env) CheckPasskeys(ctx context.Context, m *Manifest) []string {
	st, err := sqlitestore.New(e.DB.DB)
	if err != nil {
		return []string{"passkeys: " + err.Error()}
	}
	byUser := map[string][]Passkey{}
	for _, p := range m.Passkeys {
		byUser[p.UserID] = append(byUser[p.UserID], p)
	}
	var bad []string
	for legacy, want := range byUser {
		id, err := ulid.Parse(e.Mapping[legacy])
		if err != nil {
			bad = append(bad, fmt.Sprintf("passkeys: user %s has no mapping", legacy))
			continue
		}
		got, err := st.WebAuthnCredentialsByUserID(ctx, id)
		if err != nil || len(got) != len(want) {
			bad = append(bad, fmt.Sprintf("passkeys: user %s has %d rows (err %v), want %d", legacy, len(got), err, len(want)))
			continue
		}
		for _, w := range want {
			found := false
			for _, g := range got {
				if bytes.Equal(g.CredentialID, w.CredentialID) {
					found = true
					if !bytes.Equal(g.PublicKey, w.PublicKey) || g.SignCount != w.SignCount {
						bad = append(bad, fmt.Sprintf("passkeys: user %s credential differs after copy", legacy))
					}
				}
			}
			if !found {
				bad = append(bad, fmt.Sprintf("passkeys: user %s credential id missing", legacy))
			}
		}
	}
	return bad
}

// CheckOAuth confirms each seeded identity is linked to the mapped user. Call
// it only when the backfill reported an OAuth identity count.
func (e *Env) CheckOAuth(ctx context.Context, m *Manifest) []string {
	var bad []string
	for _, o := range m.OAuth {
		var n int
		err := e.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM theauth_oauth_accounts WHERE provider = ? AND provider_user_id = ? AND user_id = ?`,
			o.Provider, o.ProviderUserID, e.Mapping[o.UserID]).Scan(&n)
		if err != nil || n != 1 {
			bad = append(bad, fmt.Sprintf("oauth: %s/%s for user %s: %d rows (err %v)", o.Provider, o.ProviderUserID, o.UserID, n, err))
		}
	}
	return bad
}

// ResolveCaseDuplicates renames each case-duplicate user so its email no
// longer collides, the pre-flight step an operator performs after the
// backfill refuses. It updates the manifest to match.
func ResolveCaseDuplicates(ctx context.Context, db *sql.DB, m *Manifest) error {
	for i := range m.Users {
		u := &m.Users[i]
		if u.Kind != KindCaseDupe {
			continue
		}
		at := strings.LastIndex(u.Email, "@")
		if at < 0 {
			at = len(u.Email)
		}
		next := u.Email[:at] + "+dupe" + u.Email[at:]
		if _, err := db.ExecContext(ctx, `UPDATE users SET email = ? WHERE id = ?`, next, u.ID); err != nil {
			return fmt.Errorf("rehearsal: rename duplicate %s: %w", u.ID, err)
		}
		u.Email = next
	}
	return nil
}
