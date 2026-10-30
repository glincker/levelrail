package apiclient

import (
	"context"
	"net/http"
)

// SystemEmailTest is the outcome of the last system email test send.
type SystemEmailTest struct {
	To      string `json:"to"`
	OK      bool   `json:"ok"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message"`
	At      string `json:"at"`
}

// SystemEmailDNS grades the sender domain's SPF, DKIM and DMARC.
type SystemEmailDNS struct {
	SPF   string `json:"spf"`
	DKIM  string `json:"dkim"`
	DMARC string `json:"dmarc"`
	Pass  bool   `json:"pass"`
}

// SystemEmailState is the system email setup; it never carries a credential.
type SystemEmailState struct {
	Enabled     bool             `json:"enabled"`
	Active      bool             `json:"active"`
	Domain      string           `json:"domain,omitempty"`
	App         string           `json:"app,omitempty"`
	FromAddress string           `json:"from_address,omitempty"`
	Submission  string           `json:"submission,omitempty"`
	TLSMode     string           `json:"tls_mode,omitempty"`
	Pinned      bool             `json:"certificate_pinned"`
	HasPrevious bool             `json:"has_previous_provider"`
	LastTest    *SystemEmailTest `json:"last_test,omitempty"`
	DNS         *SystemEmailDNS  `json:"dns,omitempty"`
	Warning     string           `json:"warning,omitempty"`
	Demo        bool             `json:"demo,omitempty"`
	Domains     []string         `json:"domains"`
	PinMismatch bool             `json:"pin_mismatch"`
	Error       string           `json:"error,omitempty"`
}

// SystemEmailStep is one step of a system email action.
type SystemEmailStep struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// SystemEmailResult is the state after an action plus its steps.
type SystemEmailResult struct {
	State SystemEmailState  `json:"state"`
	Steps []SystemEmailStep `json:"steps"`
}

// SystemEmailEnableRequest is the body of POST /api/v1/mail/system-email/enable.
type SystemEmailEnableRequest struct {
	Domain          string       `json:"domain"`
	To              string       `json:"to,omitempty"`
	Port            int          `json:"port,omitempty"`
	ReplaceExisting bool         `json:"replace_existing"`
	StepUp          *StepUpProof `json:"step_up,omitempty"`
}

// SystemEmail calls GET /api/v1/mail/system-email.
func (c *Client) SystemEmail(ctx context.Context) (SystemEmailState, error) {
	var out SystemEmailState
	return out, c.do(ctx, http.MethodGet, "/api/v1/mail/system-email", nil, &out)
}

// EnableSystemEmail calls POST /api/v1/mail/system-email/enable.
func (c *Client) EnableSystemEmail(ctx context.Context, req SystemEmailEnableRequest) (SystemEmailResult, error) {
	var out SystemEmailResult
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/system-email/enable", req, &out)
}

// SystemEmailAction calls POST /api/v1/mail/system-email/{disable|rotate|test}.
func (c *Client) SystemEmailAction(ctx context.Context, action, to string, proof *StepUpProof) (SystemEmailResult, error) {
	var out SystemEmailResult
	body := map[string]any{"to": to, "step_up": proof}
	return out, c.do(ctx, http.MethodPost, "/api/v1/mail/system-email/"+action, body, &out)
}
