package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// MailPort is one row of GET /api/v1/mail/ports.
type MailPort struct {
	Port      int    `json:"port"`
	Purpose   string `json:"purpose"`
	Published bool   `json:"published"`
	Bind      string `json:"bind"`
	Firewall  string `json:"firewall"`
}

// MailPorts is the response of GET /api/v1/mail/ports.
type MailPorts struct {
	App           string     `json:"app"`
	Installed     bool       `json:"installed"`
	Ports         []MailPort `json:"ports"`
	WebListener   string     `json:"web_listener"`
	Port25Note    string     `json:"port_25_note"`
	Warning       string     `json:"warning"`
	NextStep      string     `json:"next_step"`
	BindScopeNote string     `json:"bind_scope_note"`
}

// MailPortsOpenResult is the response of POST /api/v1/mail/ports/open.
type MailPortsOpenResult struct {
	App     string `json:"app"`
	Changed []int  `json:"changed"`
	Result  string `json:"result"`
	Warning string `json:"warning"`
	Pending []int  `json:"pending,omitempty"`
	Message string `json:"message"`
}

// MailPorts calls GET /api/v1/mail/ports; app picks one install when several exist.
func (c *Client) MailPorts(ctx context.Context, app string) (MailPorts, error) {
	var out MailPorts
	path := "/api/v1/mail/ports"
	if app != "" {
		path += "?app=" + url.QueryEscape(app)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// OpenMailPorts calls POST /api/v1/mail/ports/open.
func (c *Client) OpenMailPorts(ctx context.Context, app string, confirm bool, proof *StepUpProof) (MailPortsOpenResult, error) {
	var out MailPortsOpenResult
	body := struct {
		App     string       `json:"app,omitempty"`
		Confirm bool         `json:"confirm"`
		StepUp  *StepUpProof `json:"step_up,omitempty"`
	}{app, confirm, proof}
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/ports/open", body, &out)
	return out, err
}
