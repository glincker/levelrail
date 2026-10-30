package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MailDomain is a mail domain one team owns.
type MailDomain struct {
	Domain    string `json:"domain"`
	Service   string `json:"service"`
	TeamID    string `json:"team_id"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

// Mailbox is a mailbox's ownership record; it never holds a password.
type Mailbox struct {
	Address      string `json:"address"`
	Domain       string `json:"domain"`
	Localpart    string `json:"localpart"`
	Service      string `json:"service"`
	TeamID       string `json:"team_id"`
	LinkedUserID string `json:"linked_user_id,omitempty"`
	Suspended    bool   `json:"suspended"`
	CreatedBy    string `json:"created_by"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// MailboxSecret is the show-once answer of create and password reset.
type MailboxSecret struct {
	Mailbox        Mailbox `json:"mailbox"`
	Password       string  `json:"password"`
	Notice         string  `json:"notice"`
	SendCapPerHour int     `json:"send_cap_per_hour"`
}

// MailAlias forwards an address to mailboxes.
type MailAlias struct {
	Address string   `json:"address"`
	Targets []string `json:"targets"`
}

// MailQueueEntry is delivery metadata for one queued message.
type MailQueueEntry struct {
	ID              int64     `json:"id"`
	Queued          time.Time `json:"queued"`
	Hold            bool      `json:"hold"`
	SenderAccount   string    `json:"sender_account"`
	RecipientDomain string    `json:"recipient_domain"`
	Attempts        int       `json:"attempts"`
	NextAttempt     time.Time `json:"next_attempt"`
	LastError       string    `json:"last_error,omitempty"`
}

// MailQueue is the response of GET /api/v1/mail/queue.
type MailQueue struct {
	App     string           `json:"app"`
	Queued  int              `json:"queued"`
	Entries []MailQueueEntry `json:"entries"`
}

// MailQueueHoldResult is the response of the queue hold and release calls.
type MailQueueHoldResult struct {
	App     string `json:"app"`
	Scope   string `json:"scope"`
	Changed int    `json:"changed"`
}

// MailDomains calls GET /api/v1/mail/domains.
func (c *Client) MailDomains(ctx context.Context) ([]MailDomain, error) {
	var out []MailDomain
	return out, c.do(ctx, http.MethodGet, "/api/v1/mail/domains", nil, &out)
}

// ClaimMailDomain calls POST /api/v1/mail/domains.
func (c *Client) ClaimMailDomain(ctx context.Context, domain, teamID, app string, proof *StepUpProof) (MailDomain, error) {
	var out MailDomain
	body := map[string]any{"domain": domain, "team_id": teamID, "app": app, "step_up": proof}
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/domains", body, &out)
}

// Mailboxes calls GET /api/v1/mail/mailboxes.
func (c *Client) Mailboxes(ctx context.Context, domain string) ([]Mailbox, error) {
	var out []Mailbox
	path := "/api/v1/mail/mailboxes"
	if domain != "" {
		path += "?domain=" + url.QueryEscape(domain)
	}
	return out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// CreateMailbox calls POST /api/v1/mail/mailboxes.
func (c *Client) CreateMailbox(ctx context.Context, address, linkedUserID string) (MailboxSecret, error) {
	var out MailboxSecret
	body := map[string]string{"address": address, "linked_user_id": linkedUserID}
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/mailboxes", body, &out)
}

func mailboxPath(address, suffix string) string {
	return "/api/v1/mail/mailboxes/" + url.PathEscape(address) + suffix
}

// SetMailboxSuspended calls POST /api/v1/mail/mailboxes/{address}/suspend or /resume.
func (c *Client) SetMailboxSuspended(ctx context.Context, address string, suspend bool) (Mailbox, error) {
	var out Mailbox
	suffix := "/resume"
	if suspend {
		suffix = "/suspend"
	}
	return out, c.do(ctx, http.MethodPost, mailboxPath(address, suffix), struct{}{}, &out)
}

// ResetMailboxPassword calls POST /api/v1/mail/mailboxes/{address}/password.
func (c *Client) ResetMailboxPassword(ctx context.Context, address string, proof *StepUpProof) (MailboxSecret, error) {
	var out MailboxSecret
	return out, c.do(ctx, http.MethodPost, mailboxPath(address, "/password"), map[string]any{"step_up": proof}, &out)
}

// DeleteMailbox calls DELETE /api/v1/mail/mailboxes/{address}.
func (c *Client) DeleteMailbox(ctx context.Context, address, confirm string, deleteMail bool, proof *StepUpProof) (struct{}, error) {
	body := map[string]any{"confirm": confirm, "delete_mail": deleteMail, "step_up": proof}
	return struct{}{}, c.do(ctx, http.MethodDelete, mailboxPath(address, ""), body, nil)
}

// MailAliases calls GET /api/v1/mail/aliases.
func (c *Client) MailAliases(ctx context.Context, domain string) ([]MailAlias, error) {
	var out []MailAlias
	return out, c.do(ctx, http.MethodGet, "/api/v1/mail/aliases?domain="+url.QueryEscape(domain), nil, &out)
}

// AddMailAlias calls POST /api/v1/mail/aliases.
func (c *Client) AddMailAlias(ctx context.Context, address string, targets []string) (MailAlias, error) {
	var out MailAlias
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/aliases", MailAlias{Address: address, Targets: targets}, &out)
}

// RemoveMailAlias calls DELETE /api/v1/mail/aliases/{address}.
func (c *Client) RemoveMailAlias(ctx context.Context, address string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/mail/aliases/"+url.PathEscape(address), nil, nil)
}

// MailQueueList calls GET /api/v1/mail/queue.
func (c *Client) MailQueueList(ctx context.Context, app string, limit int) (MailQueue, error) {
	var out MailQueue
	q := url.Values{}
	if app != "" {
		q.Set("app", app)
	}
	if limit > 0 {
		q.Set("max", strconv.Itoa(limit))
	}
	return out, c.do(ctx, http.MethodGet, "/api/v1/mail/queue?"+q.Encode(), nil, &out)
}

// MailQueueHold calls POST /api/v1/mail/queue/hold or /release. No ids means the whole queue.
func (c *Client) MailQueueHold(ctx context.Context, app string, ids []int64, hold bool, proof *StepUpProof) (MailQueueHoldResult, error) {
	var out MailQueueHoldResult
	path := "/api/v1/mail/queue/release"
	if hold {
		path = "/api/v1/mail/queue/hold"
	}
	body := map[string]any{"app": app, "ids": ids, "step_up": proof}
	return out, c.do(ctx, http.MethodPost, path, body, &out)
}
