package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// MailCertStatus is the response of GET /api/v1/mail/cert.
type MailCertStatus struct {
	App           string `json:"app"`
	Hostname      string `json:"hostname"`
	Source        string `json:"source"`
	Served        bool   `json:"served"`
	Subject       string `json:"subject,omitempty"`
	Issuer        string `json:"issuer,omitempty"`
	NotAfter      string `json:"not_after,omitempty"`
	DaysLeft      int    `json:"days_left"`
	SelfSigned    bool   `json:"self_signed"`
	Serial        string `json:"serial,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	ProbeError    string `json:"probe_error,omitempty"`
	RestartWindow string `json:"restart_window"`
	NextStep      string `json:"next_step"`
}

// MailCertUploadResult is the response of PUT /api/v1/mail/cert.
type MailCertUploadResult struct {
	App          string `json:"app"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message"`
	Serial       string `json:"serial"`
	SHA256       string `json:"sha256"`
	NotAfter     string `json:"not_after"`
	DaysLeft     int    `json:"days_left"`
	PreviousKept bool   `json:"previous_kept"`
}

// MailCertStatus calls GET /api/v1/mail/cert; app picks one install when several exist.
func (c *Client) MailCertStatus(ctx context.Context, app string) (MailCertStatus, error) {
	var out MailCertStatus
	p := "/api/v1/mail/cert"
	if app != "" {
		p += "?app=" + url.QueryEscape(app)
	}
	err := c.do(ctx, http.MethodGet, p, nil, &out)
	return out, err
}

// UploadMailCert calls PUT /api/v1/mail/cert with a PEM chain and its unencrypted key.
func (c *Client) UploadMailCert(ctx context.Context, app, certPEM, keyPEM string, restartNow bool, proof *StepUpProof) (MailCertUploadResult, error) {
	var out MailCertUploadResult
	body := struct {
		App        string       `json:"app,omitempty"`
		Cert       string       `json:"cert"`
		Key        string       `json:"key"`
		RestartNow bool         `json:"restart_now,omitempty"`
		StepUp     *StepUpProof `json:"step_up,omitempty"`
	}{app, certPEM, keyPEM, restartNow, proof}
	err := c.do(ctx, http.MethodPut, "/api/v1/mail/cert", body, &out)
	return out, err
}

// ObtainMailCert calls POST /api/v1/mail/cert/obtain, which is not yet implemented on the server.
func (c *Client) ObtainMailCert(ctx context.Context, app string) error {
	body := struct {
		App string `json:"app,omitempty"`
	}{app}
	return c.do(ctx, http.MethodPost, "/api/v1/mail/cert/obtain", body, nil)
}
