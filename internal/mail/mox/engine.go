package mox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mail"
)

// Domains lists served domains. mox returns either plain names or objects
// with an ASCII form; both decode here.
func (c *Client) Domains(ctx context.Context) ([]mail.Domain, error) {
	var raw []json.RawMessage
	if err := c.call(ctx, "Domains", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]mail.Domain, 0, len(raw))
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, mail.Domain{Name: s})
			continue
		}
		var o struct{ ASCII, Name string }
		if json.Unmarshal(r, &o) == nil {
			n := o.ASCII
			if n == "" {
				n = o.Name
			}
			out = append(out, mail.Domain{Name: n})
		}
	}
	return out, nil
}

// AddDomain creates the domain. With an existing account the localpart must be empty.
func (c *Client) AddDomain(ctx context.Context, d mail.DomainSpec) error {
	if d.Name == "" || d.Account == "" {
		return fmt.Errorf("%w: domain name and account are required", mail.ErrInvalid)
	}
	return c.call(ctx, "DomainAdd", []any{false, d.Name, d.Account, d.Localpart}, nil)
}

// RemoveDomain deletes the domain.
func (c *Client) RemoveDomain(ctx context.Context, name string) error {
	return c.call(ctx, "DomainRemove", []any{name}, nil)
}

// DNSRecords returns the engine's zone-file lines for the domain, parsed.
func (c *Client) DNSRecords(ctx context.Context, domain string) ([]mail.DNSRecord, error) {
	var lines []string
	if err := c.call(ctx, "DomainRecords", []any{domain}, &lines); err != nil {
		return nil, err
	}
	return ParseRecords(lines), nil
}

// DKIMKeys returns the selectors and public keys from the domain records.
func (c *Client) DKIMKeys(ctx context.Context, domain string) ([]mail.DKIMKey, error) {
	recs, err := c.DNSRecords(ctx, domain)
	if err != nil {
		return nil, err
	}
	return ExtractDKIM(recs), nil
}

// Accounts lists account names; this call is not covered by the verified
// protocol notes and is decoded defensively.
func (c *Client) Accounts(ctx context.Context) ([]mail.Account, error) {
	var names []string
	if err := c.call(ctx, "AccountNames", nil, &names); err != nil {
		return nil, err
	}
	out := make([]mail.Account, 0, len(names))
	for _, n := range names {
		out = append(out, mail.Account{Name: n})
	}
	return out, nil
}

// AddAccount creates the account and, when a password is given, sets it.
func (c *Client) AddAccount(ctx context.Context, a mail.AccountSpec) error {
	if a.Name == "" || a.Address == "" {
		return fmt.Errorf("%w: account name and address are required", mail.ErrInvalid)
	}
	if err := c.call(ctx, "AccountAdd", []any{a.Name, a.Address}, nil); err != nil {
		return err
	}
	if a.Password == "" {
		return nil
	}
	return c.SetPassword(ctx, a.Name, a.Password)
}

// RemoveAccount deletes the account by name.
func (c *Client) RemoveAccount(ctx context.Context, name string) error {
	return c.call(ctx, "AccountRemove", []any{name}, nil)
}

// AddAddress attaches another address to an existing account.
func (c *Client) AddAddress(ctx context.Context, address, account string) error {
	return c.call(ctx, "AddressAdd", []any{address, account}, nil)
}

// SetPassword sets the mailbox password; the value is scrubbed from errors.
func (c *Client) SetPassword(ctx context.Context, account, password string) error {
	return c.call(ctx, "SetPassword", []any{account, password}, nil, password)
}

// SetQuota is unsupported: mox keeps quota in its config file, not the API.
func (c *Client) SetQuota(context.Context, string, mail.Quota) error {
	return fmt.Errorf("%w: quota is a config file setting", mail.ErrUnsupported)
}

func splitAddress(addr string) (local, domain string, err error) {
	i := strings.LastIndexByte(addr, '@')
	if i <= 0 || i == len(addr)-1 {
		return "", "", fmt.Errorf("%w: %q is not an address", mail.ErrInvalid, addr)
	}
	return addr[:i], addr[i+1:], nil
}

// AddAlias forwards the alias address to its targets.
func (c *Client) AddAlias(ctx context.Context, a mail.Alias) error {
	local, domain, err := splitAddress(a.Address)
	if err != nil {
		return err
	}
	if len(a.Targets) == 0 {
		return fmt.Errorf("%w: alias needs at least one target", mail.ErrInvalid)
	}
	cfg := map[string]any{"Addresses": a.Targets, "PostPublic": false, "ListMembers": false, "AllowMsgFrom": false}
	return c.call(ctx, "AliasAdd", []any{local, domain, cfg}, nil)
}

// RemoveAlias deletes the alias.
func (c *Client) RemoveAlias(ctx context.Context, address string) error {
	local, domain, err := splitAddress(address)
	if err != nil {
		return err
	}
	return c.call(ctx, "AliasRemove", []any{local, domain}, nil)
}

// QueueStats counts queued messages and those that already failed an attempt.
func (c *Client) QueueStats(ctx context.Context) (mail.QueueStats, error) {
	var st mail.QueueStats
	if err := c.call(ctx, "QueueSize", nil, &st.Queued); err != nil {
		return st, err
	}
	if st.Queued == 0 {
		return st, nil
	}
	var items []struct {
		Attempts  int
		LastError string
	}
	if err := c.call(ctx, "QueueList", []any{map[string]any{}, map[string]any{}}, &items); err != nil {
		return st, err
	}
	for _, it := range items {
		if it.Attempts > 0 && it.LastError != "" {
			st.Failing++
		}
	}
	return st, nil
}

// HoldQueue pauses delivery of every queued message (mox QueueHoldSet).
func (c *Client) HoldQueue(ctx context.Context) error {
	return c.call(ctx, "QueueHoldSet", []any{map[string]any{}, true}, nil)
}

// ReleaseQueue resumes delivery of held messages.
func (c *Client) ReleaseQueue(ctx context.Context) error {
	return c.call(ctx, "QueueHoldSet", []any{map[string]any{}, false}, nil)
}

// Health checks login and a cheap read.
func (c *Client) Health(ctx context.Context) error {
	return c.call(ctx, "QueueSize", nil, new(int))
}
