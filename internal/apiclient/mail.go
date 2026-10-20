package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// MailToken is one substitutable email token.
type MailToken struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Sample      string `json:"sample"`
	Scope       string `json:"scope"`
}

// MailTemplateBody is the editable part of an email template.
type MailTemplateBody struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Body      string `json:"body"`
}

// MailTemplate is one event template with its current text.
type MailTemplate struct {
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Description string           `json:"description"`
	Category    string           `json:"category"`
	Tokens      []string         `json:"tokens"`
	Default     MailTemplateBody `json:"default"`
	Current     MailTemplateBody `json:"current"`
	Customized  bool             `json:"customized"`
	UpdatedAt   string           `json:"updated_at"`
}

// MailPreview is a rendered sample message.
type MailPreview struct {
	Subject   string `json:"subject"`
	Preheader string `json:"preheader"`
	Text      string `json:"text"`
	HTML      string `json:"html"`
}

// MailRules selects the recipients of a group.
type MailRules struct {
	AllUsers         bool     `json:"all_users"`
	Roles            []string `json:"roles"`
	Teams            []string `json:"teams"`
	Projects         []string `json:"projects"`
	EmailDomains     []string `json:"email_domains"`
	InactiveDays     int      `json:"inactive_days"`
	ExtraAddresses   []string `json:"extra_addresses"`
	ExcludeAddresses []string `json:"exclude_addresses"`
}

// MailGroup is a saved audience with its live recipient count.
type MailGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Rules       MailRules `json:"rules"`
	Recipients  int       `json:"recipients"`
	Suppressed  int       `json:"suppressed"`
}

// MailAudiencePreview is a live count plus a masked sample.
type MailAudiencePreview struct {
	Total       int      `json:"total"`
	Deliverable int      `json:"deliverable"`
	Suppressed  int      `json:"suppressed"`
	Sample      []string `json:"sample"`
}

// MailBroadcastRequest composes a broadcast; DryRun only reports the plan.
type MailBroadcastRequest struct {
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	TemplateKey  string   `json:"template_key,omitempty"`
	Audience     string   `json:"audience,omitempty"`
	Recipients   []string `json:"recipients,omitempty"`
	ScheduleAt   string   `json:"schedule_at,omitempty"`
	DryRun       bool     `json:"dry_run,omitempty"`
	ExpectedSend int      `json:"expected_recipients,omitempty"`
}

// MailBroadcastPlan is the dry-run answer.
type MailBroadcastPlan struct {
	MailAudiencePreview
	CapRemaining       int  `json:"cap_remaining"`
	ProviderConfigured bool `json:"provider_configured"`
}

// MailBroadcast is a sent or queued message and its progress.
type MailBroadcast struct {
	ID          string `json:"id"`
	Subject     string `json:"subject"`
	Status      string `json:"status"`
	ScheduledAt string `json:"scheduled_at"`
	Total       int    `json:"total"`
	Sent        int    `json:"sent"`
	Failed      int    `json:"failed"`
	Suppressed  int    `json:"suppressed"`
	Error       string `json:"error"`
	CreatedAt   string `json:"created_at"`
	FinishedAt  string `json:"finished_at"`
}

// ListMailTokens calls GET /api/v1/mail/tokens.
func (c *Client) ListMailTokens(ctx context.Context) ([]MailToken, error) {
	var out struct {
		Tokens []MailToken `json:"tokens"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/tokens", nil, &out)
	return out.Tokens, err
}

// ListMailTemplates calls GET /api/v1/mail/templates.
func (c *Client) ListMailTemplates(ctx context.Context) ([]MailTemplate, error) {
	var out struct {
		Templates []MailTemplate `json:"templates"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/templates", nil, &out)
	return out.Templates, err
}

// GetMailTemplate calls GET /api/v1/mail/templates/{key}.
func (c *Client) GetMailTemplate(ctx context.Context, key string) (MailTemplate, error) {
	var out MailTemplate
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/templates/"+PathEscape(key), nil, &out)
	return out, err
}

// SetMailTemplate calls PUT /api/v1/mail/templates/{key}.
func (c *Client) SetMailTemplate(ctx context.Context, key string, t MailTemplateBody) (MailTemplate, error) {
	var out MailTemplate
	err := c.do(ctx, http.MethodPut, "/api/v1/mail/templates/"+PathEscape(key), t, &out)
	return out, err
}

// ResetMailTemplate calls DELETE /api/v1/mail/templates/{key}.
func (c *Client) ResetMailTemplate(ctx context.Context, key string) (MailTemplate, error) {
	var out MailTemplate
	err := c.do(ctx, http.MethodDelete, "/api/v1/mail/templates/"+PathEscape(key), nil, &out)
	return out, err
}

// PreviewMailTemplate calls POST /api/v1/mail/templates/{key}/preview with sample data.
func (c *Client) PreviewMailTemplate(ctx context.Context, key, scheme string) (MailPreview, error) {
	var out MailPreview
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/templates/"+PathEscape(key)+"/preview", map[string]string{"scheme": scheme}, &out)
	return out, err
}

// ListMailGroups calls GET /api/v1/mail/groups.
func (c *Client) ListMailGroups(ctx context.Context) ([]MailGroup, error) {
	var out struct {
		Groups []MailGroup `json:"groups"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/groups", nil, &out)
	return out.Groups, err
}

// MailGroupDetail is a group with its masked recipient preview.
type MailGroupDetail struct {
	Group   MailGroup           `json:"group"`
	Preview MailAudiencePreview `json:"preview"`
}

// GetMailGroup calls GET /api/v1/mail/groups/{id}; id may be the group name.
func (c *Client) GetMailGroup(ctx context.Context, id string) (MailGroupDetail, error) {
	var out MailGroupDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/groups/"+PathEscape(id), nil, &out)
	return out, err
}

// CreateMailGroup calls POST /api/v1/mail/groups.
func (c *Client) CreateMailGroup(ctx context.Context, name, description string, rules MailRules) (MailGroup, error) {
	var out MailGroup
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/groups", map[string]any{"name": name, "description": description, "rules": rules}, &out)
	return out, err
}

// DeleteMailGroup calls DELETE /api/v1/mail/groups/{id}.
func (c *Client) DeleteMailGroup(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/mail/groups/"+PathEscape(id), nil, nil)
}

// CreateMailBroadcast calls POST /api/v1/mail/broadcasts for a real send.
func (c *Client) CreateMailBroadcast(ctx context.Context, req MailBroadcastRequest) (MailBroadcast, error) {
	var out MailBroadcast
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/broadcasts", req, &out)
	return out, err
}

// PlanMailBroadcast calls POST /api/v1/mail/broadcasts with dry_run set.
func (c *Client) PlanMailBroadcast(ctx context.Context, req MailBroadcastRequest) (MailBroadcastPlan, error) {
	req.DryRun = true
	var out MailBroadcastPlan
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/broadcasts", req, &out)
	return out, err
}

// GetMailBroadcast calls GET /api/v1/mail/broadcasts/{id}.
func (c *Client) GetMailBroadcast(ctx context.Context, id string) (MailBroadcast, error) {
	var out struct {
		Broadcast MailBroadcast `json:"broadcast"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/mail/broadcasts/"+PathEscape(id), nil, &out)
	return out.Broadcast, err
}

// MailGroupCountLabel formats a group's recipient count for plain output.
func MailGroupCountLabel(g MailGroup) string {
	return strconv.Itoa(g.Recipients) + " (" + strconv.Itoa(g.Suppressed) + " suppressed)"
}
