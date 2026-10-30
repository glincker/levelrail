package mox

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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

// Accounts lists accounts. mox returns two lists: every account, then the
// ones with login disabled (verified against v0.0.17).
func (c *Client) Accounts(ctx context.Context) ([]mail.Account, error) {
	var res [2][]string
	if err := c.call(ctx, "Accounts", nil, &res); err != nil {
		return nil, err
	}
	disabled := make(map[string]bool, len(res[1]))
	for _, n := range res[1] {
		disabled[n] = true
	}
	out := make([]mail.Account, 0, len(res[0]))
	for _, n := range res[0] {
		out = append(out, mail.Account{Name: n, Disabled: disabled[n]})
	}
	return out, nil
}

// AddAccount creates the account, turns off self-chosen passwords so only
// generated ones are used (ADR 059 section 2.4), and sets the password. A
// failure after the add removes the half-made account again.
func (c *Client) AddAccount(ctx context.Context, a mail.AccountSpec) error {
	if a.Name == "" || a.Address == "" {
		return fmt.Errorf("%w: account name and address are required", mail.ErrInvalid)
	}
	if err := c.call(ctx, "AccountAdd", []any{a.Name, a.Address}, nil); err != nil {
		return err
	}
	err := c.call(ctx, "AccountSettingsSave", []any{a.Name, 0, 0, 0, true, true}, nil)
	if err == nil && a.Password != "" {
		err = c.SetPassword(ctx, a.Name, a.Password)
	}
	if err != nil {
		_ = c.call(ctx, "AccountRemove", []any{a.Name}, nil)
		return err
	}
	return nil
}

// AccountDetail reads the account config and its disk usage; mox returns both
// as a two element result.
func (c *Client) AccountDetail(ctx context.Context, name string) (mail.AccountDetail, error) {
	var res []json.RawMessage
	if err := c.call(ctx, "Account", []any{name}, &res); err != nil {
		return mail.AccountDetail{}, err
	}
	if len(res) != 2 {
		return mail.AccountDetail{}, fmt.Errorf("mail: unexpected Account result")
	}
	var acc struct {
		LoginDisabled string
		Destinations  map[string]json.RawMessage
	}
	var usage int64
	if err := json.Unmarshal(res[0], &acc); err != nil {
		return mail.AccountDetail{}, fmt.Errorf("mail: decode Account result: %w", err)
	}
	if err := json.Unmarshal(res[1], &usage); err != nil {
		return mail.AccountDetail{}, fmt.Errorf("mail: decode Account usage: %w", err)
	}
	out := mail.AccountDetail{Name: name, LoginDisabled: acc.LoginDisabled != "", DiskUsageBytes: usage, Addresses: make([]string, 0, len(acc.Destinations))}
	for addr := range acc.Destinations {
		out.Addresses = append(out.Addresses, addr)
	}
	sort.Strings(out.Addresses)
	return out, nil
}

// SetLoginDisabled saves the account's LoginDisabled message; "" re-enables.
func (c *Client) SetLoginDisabled(ctx context.Context, account, message string) error {
	return c.call(ctx, "AccountLoginDisabledSave", []any{account, message}, nil)
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

// queueMsg decodes only delivery metadata from a mox queue message; Subject,
// MessageID and MsgPrefix are never read.
type queueMsg struct {
	ID                 int64
	Queued             time.Time
	Hold               bool
	SenderAccount      string
	RecipientDomainStr string
	Attempts           int
	NextAttempt        time.Time
	Results            []struct {
		Success bool
		Error   string
	}
}

func (m queueMsg) lastError() string {
	if n := len(m.Results); n > 0 && !m.Results[n-1].Success {
		return m.Results[n-1].Error
	}
	return ""
}

func (c *Client) queueMsgs(ctx context.Context, limit int) ([]queueMsg, error) {
	var items []queueMsg
	filter := map[string]any{}
	if limit > 0 {
		filter["Max"] = limit
	}
	err := c.call(ctx, "QueueList", []any{filter, map[string]any{"Field": "Queued", "Asc": true}}, &items)
	return items, err
}

// Aliases lists a domain's aliases from DomainLocalparts, whose second
// result maps alias localparts to their config.
func (c *Client) Aliases(ctx context.Context, domain string) ([]mail.Alias, error) {
	var res []json.RawMessage
	if err := c.call(ctx, "DomainLocalparts", []any{domain}, &res); err != nil {
		return nil, err
	}
	if len(res) != 2 {
		return nil, fmt.Errorf("mail: unexpected DomainLocalparts result")
	}
	var raw map[string]struct{ Addresses []string }
	if err := json.Unmarshal(res[1], &raw); err != nil {
		return nil, fmt.Errorf("mail: decode aliases: %w", err)
	}
	out := make([]mail.Alias, 0, len(raw))
	for lp, a := range raw {
		out = append(out, mail.Alias{Address: lp + "@" + domain, Targets: a.Addresses})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

// QueueStats counts queued messages and those whose last attempt failed.
func (c *Client) QueueStats(ctx context.Context) (mail.QueueStats, error) {
	var st mail.QueueStats
	if err := c.call(ctx, "QueueSize", nil, &st.Queued); err != nil {
		return st, err
	}
	if st.Queued == 0 {
		return st, nil
	}
	items, err := c.queueMsgs(ctx, 0)
	if err != nil {
		return st, err
	}
	for _, it := range items {
		if it.Attempts > 0 && it.lastError() != "" {
			st.Failing++
		}
	}
	return st, nil
}

// QueueList lists up to limit queued messages, oldest first.
func (c *Client) QueueList(ctx context.Context, limit int) ([]mail.QueueEntry, error) {
	items, err := c.queueMsgs(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mail.QueueEntry, 0, len(items))
	for _, it := range items {
		out = append(out, mail.QueueEntry{ID: it.ID, Queued: it.Queued, Hold: it.Hold, SenderAccount: it.SenderAccount,
			RecipientDomain: it.RecipientDomainStr, Attempts: it.Attempts, NextAttempt: it.NextAttempt, LastError: it.lastError()})
	}
	return out, nil
}

// HoldMessages sets hold on the listed ids only. An empty list is refused
// because an empty mox filter matches the whole queue.
func (c *Client) HoldMessages(ctx context.Context, ids []int64, hold bool) (int, error) {
	if len(ids) == 0 {
		return 0, fmt.Errorf("%w: list at least one queue id", mail.ErrInvalid)
	}
	var n int
	err := c.call(ctx, "QueueHoldSet", []any{map[string]any{"IDs": ids}, hold}, &n)
	return n, err
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
