// Package mailgroups defines email audiences: saved rules over platform
// users plus manual addresses, with a suppression list and signed
// one-click unsubscribe tokens.
package mailgroups

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"
)

// Roles a rule can select.
var Roles = []string{"owner", "admin", "operator", "viewer"}

// maxExtra bounds the manual address list a group may carry.
const maxExtra = 5000

// Rules selects recipients. A user is included when any selector matches
// (AllUsers, Roles, Teams, Projects, EmailDomains); InactiveDays then
// narrows the result, and alone selects every user inactive that long.
// ExtraAddresses are added as-is; ExcludeAddresses are always removed.
type Rules struct {
	AllUsers         bool     `json:"all_users"`
	Roles            []string `json:"roles"`
	Teams            []string `json:"teams"`
	Projects         []string `json:"projects"`
	EmailDomains     []string `json:"email_domains"`
	InactiveDays     int      `json:"inactive_days"`
	ExtraAddresses   []string `json:"extra_addresses"`
	ExcludeAddresses []string `json:"exclude_addresses"`
}

// User is the slice of a platform account the rules look at.
type User struct {
	ID          string
	Email       string
	DisplayName string
	Role        string
	TeamIDs     []string
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

// Directory is everything Resolve needs to know about the instance.
type Directory struct {
	Users []User
	// ProjectTeams maps a project id onto the team that owns it.
	ProjectTeams map[string]string
}

// Recipient is one resolved address.
type Recipient struct {
	Email       string `json:"email"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// Normalize lowercases and trims an address.
func Normalize(addr string) string { return strings.ToLower(strings.TrimSpace(addr)) }

// Validate checks rules are well formed.
func (r Rules) Validate() error {
	for _, role := range r.Roles {
		if !contains(Roles, role) {
			return fmt.Errorf("unknown role %q", role)
		}
	}
	if r.InactiveDays < 0 || r.InactiveDays > 3650 {
		return errors.New("inactive_days must be between 0 and 3650")
	}
	if len(r.ExtraAddresses) > maxExtra {
		return fmt.Errorf("at most %d extra addresses", maxExtra)
	}
	for _, a := range append(append([]string{}, r.ExtraAddresses...), r.ExcludeAddresses...) {
		if err := ValidAddress(a); err != nil {
			return err
		}
	}
	for _, d := range r.EmailDomains {
		if d == "" || strings.ContainsAny(d, "@ ") || !strings.Contains(d, ".") {
			return fmt.Errorf("%q is not an email domain", d)
		}
	}
	return nil
}

// ValidAddress reports an error unless a is one bare email address.
func ValidAddress(a string) error {
	p, err := mail.ParseAddress(a)
	if err != nil || p.Address != strings.TrimSpace(a) {
		return fmt.Errorf("%q is not a valid email address", a)
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Resolve returns the deduplicated recipients the rules select, sorted by
// address. Suppression is applied separately by Split.
func Resolve(d Directory, r Rules, now time.Time) []Recipient {
	projectTeams := map[string]bool{}
	for _, p := range r.Projects {
		if t, ok := d.ProjectTeams[p]; ok {
			projectTeams[t] = true
		}
	}
	selectors := r.AllUsers || len(r.Roles) > 0 || len(r.Teams) > 0 || len(r.Projects) > 0 || len(r.EmailDomains) > 0
	exclude := map[string]bool{}
	for _, a := range r.ExcludeAddresses {
		exclude[Normalize(a)] = true
	}
	seen := map[string]Recipient{}
	for _, u := range d.Users {
		if !userSelected(u, r, projectTeams, selectors) || !inactiveEnough(u, r.InactiveDays, now) {
			continue
		}
		email := Normalize(u.Email)
		if email == "" || !strings.Contains(email, "@") || exclude[email] {
			continue
		}
		seen[email] = Recipient{Email: email, UserID: u.ID, DisplayName: u.DisplayName, Role: u.Role}
	}
	byEmail := map[string]User{}
	for _, u := range d.Users {
		byEmail[Normalize(u.Email)] = u
	}
	for _, a := range r.ExtraAddresses {
		email := Normalize(a)
		if _, ok := seen[email]; ok || exclude[email] {
			continue
		}
		rc := Recipient{Email: email}
		if u, ok := byEmail[email]; ok {
			rc.UserID, rc.DisplayName, rc.Role = u.ID, u.DisplayName, u.Role
		}
		seen[email] = rc
	}
	out := make([]Recipient, 0, len(seen))
	for _, rc := range seen {
		out = append(out, rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out
}

func userSelected(u User, r Rules, projectTeams map[string]bool, selectors bool) bool {
	if !selectors {
		return r.InactiveDays > 0
	}
	if r.AllUsers || contains(r.Roles, u.Role) {
		return true
	}
	for _, t := range u.TeamIDs {
		if contains(r.Teams, t) || projectTeams[t] {
			return true
		}
	}
	if at := strings.LastIndex(u.Email, "@"); at >= 0 {
		domain := strings.ToLower(u.Email[at+1:])
		for _, d := range r.EmailDomains {
			if strings.ToLower(d) == domain {
				return true
			}
		}
	}
	return false
}

func inactiveEnough(u User, days int, now time.Time) bool {
	if days <= 0 {
		return true
	}
	cutoff := now.AddDate(0, 0, -days)
	if u.LastLoginAt != nil {
		return u.LastLoginAt.Before(cutoff)
	}
	return u.CreatedAt.Before(cutoff)
}

// Split separates suppressed recipients from deliverable ones. When honor
// is false (system mail) nobody is suppressed.
func Split(rcpts []Recipient, suppressed map[string]bool, honor bool) (send, held []Recipient) {
	for _, r := range rcpts {
		if honor && suppressed[r.Email] {
			held = append(held, r)
			continue
		}
		send = append(send, r)
	}
	return send, held
}

// Mask hides most of an address for previews: "s***@example.com".
func Mask(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}

const unsubDomain = "mail-unsubscribe\x00"

// SignUnsubscribe returns a token naming email that only this instance can
// have produced. It never expires: an unsubscribe link must keep working.
func SignUnsubscribe(key []byte, email string) string {
	email = Normalize(email)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(unsubDomain + email))
	return base64.RawURLEncoding.EncodeToString([]byte(email)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// VerifyUnsubscribe returns the address a token names when its signature holds.
func VerifyUnsubscribe(key []byte, token string) (string, bool) {
	if len(token) > 600 {
		return "", false
	}
	enc, _, ok := strings.Cut(token, ".")
	if !ok {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	email := string(raw)
	if subtle.ConstantTimeCompare([]byte(SignUnsubscribe(key, email)), []byte(token)) != 1 {
		return "", false
	}
	return Normalize(email), true
}
