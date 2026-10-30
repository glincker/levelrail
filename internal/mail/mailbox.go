package mail

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// PasswordLengthEnv sets the length of generated mailbox passwords.
const (
	PasswordLengthEnv     = "APP_MAIL_PASSWORD_LENGTH" //nolint:gosec // env var name, not a credential
	defaultPasswordLength = 24
	minPasswordLength     = 16
	// SuspendedMessage is what the engine tells a client that tries to log in.
	SuspendedMessage = "This mailbox is suspended. Ask your team admin."
)

// ErrHasMail refuses a delete while the account still holds mail.
var ErrHasMail = errors.New("mail: the mailbox still holds mail")

// readableRunes drops look-alike characters so a password read aloud survives.
var readableRunes = lettersExcept("abcdefghijklmnopqrstuvwxyz", "lo") + lettersExcept("ABCDEFGHIJKLMNOPQRSTUVWXYZ", "IO") + lettersExcept("0123456789", "01")

func lettersExcept(set, drop string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(drop, r) {
			return -1
		}
		return r
	}, set)
}

// PasswordLength reads the generated password length, never below the minimum.
func PasswordLength() int {
	if n, err := strconv.Atoi(os.Getenv(PasswordLengthEnv)); err == nil && n >= minPasswordLength {
		return n
	}
	return defaultPasswordLength
}

// GeneratePassword returns a random password from crypto/rand.
func GeneratePassword() (string, error) {
	n := PasswordLength()
	var b strings.Builder
	size := big.NewInt(int64(len(readableRunes)))
	for range n {
		i, err := rand.Int(rand.Reader, size)
		if err != nil {
			return "", fmt.Errorf("mail: generate password: %w", err)
		}
		b.WriteByte(readableRunes[i.Int64()])
	}
	return b.String(), nil
}

// AccountNameFor derives the engine account name from an address. Names are
// the full lowercase address so two domains never collide.
func AccountNameFor(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// CreateAccount creates the account with a generated password and returns the
// password once. The caller shows it to the admin and never stores it.
func CreateAccount(ctx context.Context, e Engine, address string, sendCapPerHour int) (string, error) {
	pw, err := GeneratePassword()
	if err != nil {
		return "", err
	}
	if sendCapPerHour <= 0 {
		sendCapPerHour = DefaultSendCapPerHour()
	}
	spec := AccountSpec{Name: AccountNameFor(address), Address: AccountNameFor(address), Password: pw, SendCapPerHour: sendCapPerHour}
	if err := e.AddAccount(ctx, spec); err != nil {
		return "", err
	}
	return pw, nil
}

// ListAccounts lists every account with its login state.
func ListAccounts(ctx context.Context, e Engine) ([]Account, error) { return e.Accounts(ctx) }

// SetAccountPassword rotates the password and returns the new one once.
func SetAccountPassword(ctx context.Context, e Engine, account string) (string, error) {
	pw, err := GeneratePassword()
	if err != nil {
		return "", err
	}
	if err := e.SetPassword(ctx, account, pw); err != nil {
		return "", err
	}
	return pw, nil
}

// SuspendAccount refuses logins and keeps receiving mail (ADR 059 section 2.5).
func SuspendAccount(ctx context.Context, e Engine, account string) error {
	return e.SetLoginDisabled(ctx, account, SuspendedMessage)
}

// ResumeAccount allows logins again.
func ResumeAccount(ctx context.Context, e Engine, account string) error {
	return e.SetLoginDisabled(ctx, account, "")
}

// DeleteAccount removes the account. confirm must repeat the account name, and
// an account that still uses disk is refused unless deleteMail is set.
func DeleteAccount(ctx context.Context, e Engine, account, confirm string, deleteMail bool) error {
	if account == "" || confirm != account {
		return fmt.Errorf("%w: type the mailbox address to confirm the delete", ErrInvalid)
	}
	d, err := e.AccountDetail(ctx, account)
	if err != nil {
		return err
	}
	if d.DiskUsageBytes > 0 && !deleteMail {
		return fmt.Errorf("%w (%d bytes); suspend it instead, or pass delete mail to remove it with its mail", ErrHasMail, d.DiskUsageBytes)
	}
	return e.RemoveAccount(ctx, account)
}

// ListAliases lists one domain's aliases.
func ListAliases(ctx context.Context, e Engine, domain string) ([]Alias, error) {
	return e.Aliases(ctx, domain)
}

// ListQueue lists queued messages, metadata only.
func ListQueue(ctx context.Context, e Engine, limit int) ([]QueueEntry, error) {
	return e.QueueList(ctx, limit)
}

// SplitAddress returns the localpart and lowercase domain of an address.
func SplitAddress(addr string) (local, domain string, err error) {
	addr = strings.TrimSpace(addr)
	i := strings.LastIndexByte(addr, '@')
	if i <= 0 || i == len(addr)-1 || strings.ContainsAny(addr, " \t\r\n/\\") {
		return "", "", fmt.Errorf("%w: %q is not an address", ErrInvalid, addr)
	}
	return addr[:i], strings.ToLower(addr[i+1:]), nil
}
