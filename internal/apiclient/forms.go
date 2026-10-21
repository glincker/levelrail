package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Form is one capture endpoint of an app.
type Form struct {
	ID               string    `json:"id"`
	App              string    `json:"app"`
	Name             string    `json:"name"`
	RedirectURL      string    `json:"redirect_url"`
	HoneypotField    string    `json:"honeypot_field"`
	NotifyChannelIDs []string  `json:"notify_channel_ids"`
	EndpointPath     string    `json:"endpoint_path"`
	Endpoint         string    `json:"endpoint"`
	Snippet          string    `json:"snippet"`
	EndpointSource   string    `json:"endpoint_source"`
	Submissions      int       `json:"submissions"`
	SpamSubmissions  int       `json:"spam_submissions"`
	CreatedAt        time.Time `json:"created_at"`
}

// FormsList is the response of listing an app's forms.
type FormsList struct {
	Forms         []Form `json:"forms"`
	RetentionDays int    `json:"retention_days"`
}

// FormRequest creates or updates a form; nil pointers leave a field as it is.
type FormRequest struct {
	Name             string   `json:"name,omitempty"`
	RedirectURL      *string  `json:"redirect_url,omitempty"`
	HoneypotField    *string  `json:"honeypot_field,omitempty"`
	NotifyChannelIDs []string `json:"notify_channel_ids,omitempty"`
}

// FormSubmission is one stored post.
type FormSubmission struct {
	ID         int64             `json:"id"`
	Fields     map[string]string `json:"fields"`
	Spam       bool              `json:"spam"`
	Test       bool              `json:"test,omitempty"`
	SpamReason string            `json:"spam_reason,omitempty"`
	IPHash     string            `json:"ip_hash,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// FormSubmissionsPage is one page of an inbox; NextBefore is 0 on the last page.
type FormSubmissionsPage struct {
	Submissions []FormSubmission `json:"submissions"`
	NextBefore  int64            `json:"next_before"`
}

// FormSubmissionFilter narrows an inbox listing or export; Spam is "", "true" or "false".
type FormSubmissionFilter struct {
	Query  string
	Spam   string
	Limit  int
	Before int64
}

func formPath(app, form string) string {
	p := "/api/v1/apps/" + PathEscape(app) + "/forms"
	if form != "" {
		p += "/" + PathEscape(form)
	}
	return p
}

func (f FormSubmissionFilter) values() string {
	v := url.Values{}
	if f.Query != "" {
		v.Set("q", f.Query)
	}
	if f.Spam != "" {
		v.Set("spam", f.Spam)
	}
	if f.Limit > 0 {
		v.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Before > 0 {
		v.Set("before", strconv.FormatInt(f.Before, 10))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// ListForms lists an app's forms.
func (c *Client) ListForms(ctx context.Context, app string) (FormsList, error) {
	var out FormsList
	err := c.do(ctx, http.MethodGet, formPath(app, ""), nil, &out)
	return out, err
}

// CreateForm adds a form to an app.
func (c *Client) CreateForm(ctx context.Context, app string, req FormRequest) (Form, error) {
	var out Form
	err := c.do(ctx, http.MethodPost, formPath(app, ""), req, &out)
	return out, err
}

// UpdateForm changes a form's redirect, honeypot field or notification channels.
func (c *Client) UpdateForm(ctx context.Context, app, form string, req FormRequest) (Form, error) {
	var out Form
	err := c.do(ctx, http.MethodPut, formPath(app, form), req, &out)
	return out, err
}

// DeleteForm removes a form and all of its submissions.
func (c *Client) DeleteForm(ctx context.Context, app, form string) error {
	return c.do(ctx, http.MethodDelete, formPath(app, form), nil, nil)
}

// ListFormSubmissions returns one page of a form's inbox.
func (c *Client) ListFormSubmissions(ctx context.Context, app, form string, f FormSubmissionFilter) (FormSubmissionsPage, error) {
	var out FormSubmissionsPage
	err := c.do(ctx, http.MethodGet, formPath(app, form)+"/submissions"+f.values(), nil, &out)
	return out, err
}

// ExportFormSubmissions downloads matching submissions as CSV.
func (c *Client) ExportFormSubmissions(ctx context.Context, app, form string, f FormSubmissionFilter) ([]byte, error) {
	return c.downloadRaw(ctx, formPath(app, form)+"/export"+f.values())
}

// MarkFormSubmissionSpam flags or clears one submission.
func (c *Client) MarkFormSubmissionSpam(ctx context.Context, app, form string, id int64, spam bool) error {
	body := struct {
		Spam bool `json:"spam"`
	}{spam}
	return c.do(ctx, http.MethodPut, formPath(app, form)+"/submissions/"+strconv.FormatInt(id, 10)+"/spam", body, nil)
}

// DeleteFormSubmission removes one submission.
func (c *Client) DeleteFormSubmission(ctx context.Context, app, form string, id int64) error {
	return c.do(ctx, http.MethodDelete, formPath(app, form)+"/submissions/"+strconv.FormatInt(id, 10), nil, nil)
}

// FormTestResult is the outcome of a test submission.
type FormTestResult struct {
	SubmissionID int64             `json:"submission_id"`
	Spam         bool              `json:"spam"`
	SpamReason   string            `json:"spam_reason,omitempty"`
	Fields       map[string]string `json:"fields"`
}

// GetFormFields returns the form's designer schema as JSON.
func (c *Client) GetFormFields(ctx context.Context, app, form string) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, formPath(app, form)+"/fields", nil, &out)
	return out, err
}

// SetFormFields replaces the form's designer schema (fields, success message, allowed origins).
func (c *Client) SetFormFields(ctx context.Context, app, form string, schema json.RawMessage) (Form, error) {
	var out Form
	err := c.do(ctx, http.MethodPut, formPath(app, form)+"/fields", schema, &out)
	return out, err
}

// SendFormTest posts a sample submission, marked as a test, through the real storage path.
func (c *Client) SendFormTest(ctx context.Context, app, form string) (FormTestResult, error) {
	var out FormTestResult
	err := c.do(ctx, http.MethodPost, formPath(app, form)+"/test", nil, &out)
	return out, err
}
