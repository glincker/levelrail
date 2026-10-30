package demo

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/mail"
)

type account struct {
	addrs    []string
	disabled bool
	disk     int64
}

// Engine is an in-memory mail.Engine seeded with fixtures. It makes no
// network or filesystem call, so nothing it holds can reach a real server.
type Engine struct {
	mu       sync.Mutex
	domains  []string
	accounts map[string]*account
	aliases  []mail.Alias
	queue    []mail.QueueEntry
}

var _ mail.Engine = (*Engine)(nil)

// NewEngine returns an engine with two domains, five mailboxes, aliases and a queue.
func NewEngine() *Engine {
	now := time.Now()
	e := &Engine{
		domains: []string{DefaultDomain, "acme.example"},
		accounts: map[string]*account{
			"alice": {addrs: []string{"alice@demo.example"}, disk: 48 << 20},
			"bob":   {addrs: []string{"bob@demo.example"}, disk: 12 << 20},
			"carol": {addrs: []string{"carol@demo.example"}, disk: 3 << 20, disabled: true},
			"dave":  {addrs: []string{"dave@acme.example"}, disk: 220 << 20},
			"erin":  {addrs: []string{"erin@acme.example"}, disk: 7 << 20},
		},
		aliases: []mail.Alias{
			{Address: "support@demo.example", Targets: []string{"alice@demo.example", "bob@demo.example"}},
			{Address: "sales@acme.example", Targets: []string{"dave@acme.example"}},
			{Address: "postmaster@demo.example", Targets: []string{"alice@demo.example"}},
		},
		queue: []mail.QueueEntry{
			{ID: 101, Queued: now.Add(-2 * time.Minute), SenderAccount: "alice", RecipientDomain: "gmail.example", NextAttempt: now.Add(time.Minute)},
			{ID: 102, Queued: now.Add(-40 * time.Minute), SenderAccount: "bob", RecipientDomain: "corp.example", Hold: true, NextAttempt: now.Add(time.Hour)},
			{ID: 103, Queued: now.Add(-3 * time.Hour), SenderAccount: "dave", RecipientDomain: "bounce.example", Attempts: 5, NextAttempt: now.Add(30 * time.Minute),
				LastError: "dial tcp 198.51.100.7:25: i/o timeout"},
			{ID: 104, Queued: now.Add(-12 * time.Minute), SenderAccount: "erin", RecipientDomain: "slow.example", Attempts: 2, NextAttempt: now.Add(5 * time.Minute),
				LastError: "451 4.7.1 greylisted, try again later"},
		},
	}
	return e
}

func notFound(what string) error {
	return &mail.Error{Kind: mail.ErrNotFound, Message: what + " not found"}
}

// Domains implements mail.Engine.
func (e *Engine) Domains(context.Context) ([]mail.Domain, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]mail.Domain, 0, len(e.domains))
	for _, d := range e.domains {
		out = append(out, mail.Domain{Name: d})
	}
	return out, nil
}

// AddDomain implements mail.Engine.
func (e *Engine) AddDomain(_ context.Context, d mail.DomainSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	name := strings.ToLower(d.Name)
	if slices.Contains(e.domains, name) {
		return &mail.Error{Kind: mail.ErrExists, Message: name}
	}
	e.domains = append(e.domains, name)
	if d.Localpart != "" && d.Account != "" {
		e.accounts[d.Account] = &account{addrs: []string{d.Localpart + "@" + name}}
	}
	return nil
}

// RemoveDomain implements mail.Engine.
func (e *Engine) RemoveDomain(_ context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := slices.Index(e.domains, strings.ToLower(name))
	if i < 0 {
		return notFound("domain")
	}
	e.domains = slices.Delete(e.domains, i, i+1)
	return nil
}

// DNSRecords implements mail.Engine.
func (e *Engine) DNSRecords(_ context.Context, domain string) ([]mail.DNSRecord, error) {
	return []mail.DNSRecord{
		{Name: domain + ".", TTL: 300, Type: "MX", Value: "10 mail." + domain + "."},
		{Name: domain + ".", TTL: 300, Type: "TXT", Value: "v=spf1 mx -all"},
	}, nil
}

// DKIMKeys implements mail.Engine.
func (e *Engine) DKIMKeys(_ context.Context, _ string) ([]mail.DKIMKey, error) {
	k := DKIMPublicKey()
	return []mail.DKIMKey{{Selector: DKIMSelector, PublicKey: k, TXT: "v=DKIM1;k=ed25519;p=" + k}}, nil
}

// Accounts implements mail.Engine.
func (e *Engine) Accounts(context.Context) ([]mail.Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []mail.Account
	for n, a := range e.accounts {
		out = append(out, mail.Account{Name: n, Disabled: a.disabled})
	}
	slices.SortFunc(out, func(a, b mail.Account) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// AddAccount implements mail.Engine.
func (e *Engine) AddAccount(_ context.Context, a mail.AccountSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.accounts[a.Name]; ok {
		return &mail.Error{Kind: mail.ErrExists, Message: a.Name}
	}
	e.accounts[a.Name] = &account{addrs: []string{strings.ToLower(a.Address)}}
	return nil
}

// RemoveAccount implements mail.Engine.
func (e *Engine) RemoveAccount(_ context.Context, name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.accounts[name]; !ok {
		return notFound("account")
	}
	delete(e.accounts, name)
	return nil
}

// AddAddress implements mail.Engine.
func (e *Engine) AddAddress(_ context.Context, address, acct string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.accounts[acct]
	if !ok {
		return notFound("account")
	}
	a.addrs = append(a.addrs, strings.ToLower(address))
	return nil
}

// SetPassword implements mail.Engine.
func (e *Engine) SetPassword(_ context.Context, acct, _ string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.accounts[acct]; !ok {
		return notFound("account")
	}
	return nil
}

// SetQuota implements mail.Engine.
func (e *Engine) SetQuota(_ context.Context, acct string, _ mail.Quota) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.accounts[acct]; !ok {
		return notFound("account")
	}
	return nil
}

// AddAlias implements mail.Engine.
func (e *Engine) AddAlias(_ context.Context, a mail.Alias) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, have := range e.aliases {
		if strings.EqualFold(have.Address, a.Address) {
			return &mail.Error{Kind: mail.ErrExists, Message: a.Address}
		}
	}
	e.aliases = append(e.aliases, mail.Alias{Address: strings.ToLower(a.Address), Targets: slices.Clone(a.Targets)})
	return nil
}

// RemoveAlias implements mail.Engine.
func (e *Engine) RemoveAlias(_ context.Context, address string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := len(e.aliases)
	e.aliases = slices.DeleteFunc(e.aliases, func(a mail.Alias) bool { return strings.EqualFold(a.Address, address) })
	if len(e.aliases) == n {
		return notFound("alias")
	}
	return nil
}

// Aliases implements mail.Engine.
func (e *Engine) Aliases(_ context.Context, domain string) ([]mail.Alias, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []mail.Alias{}
	for _, a := range e.aliases {
		if strings.HasSuffix(strings.ToLower(a.Address), "@"+strings.ToLower(domain)) {
			out = append(out, a)
		}
	}
	return out, nil
}

// AccountDetail implements mail.Engine.
func (e *Engine) AccountDetail(_ context.Context, name string) (mail.AccountDetail, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.accounts[name]
	if !ok {
		return mail.AccountDetail{}, notFound("account")
	}
	return mail.AccountDetail{Name: name, Addresses: slices.Clone(a.addrs), LoginDisabled: a.disabled, DiskUsageBytes: a.disk}, nil
}

// SetLoginDisabled implements mail.Engine.
func (e *Engine) SetLoginDisabled(_ context.Context, acct, message string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.accounts[acct]
	if !ok {
		return notFound("account")
	}
	a.disabled = message != ""
	return nil
}

// QueueList implements mail.Engine.
func (e *Engine) QueueList(_ context.Context, limit int) ([]mail.QueueEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := slices.Clone(e.queue)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// HoldMessages implements mail.Engine.
func (e *Engine) HoldMessages(_ context.Context, ids []int64, hold bool) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for i := range e.queue {
		if slices.Contains(ids, e.queue[i].ID) && e.queue[i].Hold != hold {
			e.queue[i].Hold = hold
			n++
		}
	}
	return n, nil
}

// QueueStats implements mail.Engine.
func (e *Engine) QueueStats(context.Context) (mail.QueueStats, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := mail.QueueStats{Queued: len(e.queue)}
	for _, q := range e.queue {
		if q.LastError != "" {
			s.Failing++
		}
	}
	return s, nil
}

func (e *Engine) setAllHold(hold bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.queue {
		e.queue[i].Hold = hold
	}
}

// HoldQueue implements mail.Engine.
func (e *Engine) HoldQueue(context.Context) error { e.setAllHold(true); return nil }

// ReleaseQueue implements mail.Engine.
func (e *Engine) ReleaseQueue(context.Context) error { e.setAllHold(false); return nil }

// Health implements mail.Engine.
func (e *Engine) Health(context.Context) error { return nil }
