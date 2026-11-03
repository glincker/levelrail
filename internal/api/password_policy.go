package api

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// passwordPolicy is the live length and character-mix rule for new passwords.
type passwordPolicy struct {
	MinLength int
	Classes   int
}

func (rt *Router) passwordPolicyNow(ctx context.Context) passwordPolicy {
	svc := rt.runtimeSettings
	if svc == nil {
		svc = runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	}
	p := passwordPolicy{
		MinLength: svc.Int(ctx, runtimesettings.KeyAuthPasswordMinLength),
		Classes:   svc.Int(ctx, runtimesettings.KeyAuthPasswordRequireClasses),
	}
	// Never weaker than the code floor, even if a stored value is out of range.
	p.MinLength = max(p.MinLength, runtimesettings.PasswordMinLengthFloor)
	p.Classes = min(max(p.Classes, 0), 4)
	return p
}

// Check returns a plain-words reason the password is refused, or "" when it passes.
// subject is how the field is named in the message, such as "password".
func (p passwordPolicy) Check(subject, password string) string {
	var problems []string
	if len(password) < p.MinLength {
		problems = append(problems, fmt.Sprintf("be at least %d characters", p.MinLength))
	}
	if p.Classes > 0 && characterClasses(password) < p.Classes {
		problems = append(problems, fmt.Sprintf("mix at least %d of: lowercase letters, uppercase letters, digits, symbols", p.Classes))
	}
	if len(problems) == 0 {
		return ""
	}
	return subject + " must " + strings.Join(problems, " and ")
}

func characterClasses(s string) int {
	var lower, upper, digit, other bool
	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	n := 0
	for _, b := range []bool{lower, upper, digit, other} {
		if b {
			n++
		}
	}
	return n
}

// passwordProblem checks a password against the live policy.
func (rt *Router) passwordProblem(ctx context.Context, subject, password string) string {
	return rt.passwordPolicyNow(ctx).Check(subject, password)
}
