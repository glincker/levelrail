// Command twilio-sms sends a short SMS for each alert through Twilio.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "twilio-sms"
	pluginVersion = "0.1.0"
	maxText       = 320
)

var (
	apiBase = "https://api.twilio.com"
	httpc   pluginkit.HTTP

	sidPattern   = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
	phonePattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)
	msidPattern  = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
)

type config struct {
	AccountSID          string `json:"account_sid"`
	From                string `json:"from"`
	MessagingServiceSID string `json:"messaging_service_sid"`
	To                  string `json:"to"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion, Send: send,
		ConfigSchema: `{"type":"object","additionalProperties":false,"required":["account_sid","to"],"properties":{"account_sid":{"type":"string","description":"Twilio account SID (AC...)"},"from":{"type":"string","description":"Sender number in E.164, or use messaging_service_sid"},"messaging_service_sid":{"type":"string","description":"Messaging service SID (MG...), alternative to from"},"to":{"type":"string","description":"Recipient number in E.164"}}}`,
	}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg config
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	if err := validate(cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	if strings.TrimSpace(c.Secret) == "" {
		return pluginkit.Invalid(errors.New("the channel target must be the Twilio auth token"))
	}
	form := url.Values{"To": {cfg.To}, "Body": {pluginkit.Truncate(pluginkit.Text(c.Event), maxText)}}
	if cfg.MessagingServiceSID != "" {
		form.Set("MessagingServiceSid", cfg.MessagingServiceSID)
	} else {
		form.Set("From", cfg.From)
	}
	payload := form.Encode()
	endpoint := apiBase + "/2010-04-01/Accounts/" + cfg.AccountSID + "/Messages.json"
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(cfg.AccountSID, c.Secret)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}, nil, c.Secret)
}

func validate(cfg config) error {
	switch {
	case !sidPattern.MatchString(cfg.AccountSID):
		return errors.New("invalid config: account_sid must look like AC followed by 32 hex characters")
	case !phonePattern.MatchString(cfg.To):
		return errors.New("invalid config: to must be an E.164 number such as +15551234567")
	case cfg.MessagingServiceSID != "" && !msidPattern.MatchString(cfg.MessagingServiceSID):
		return errors.New("invalid config: messaging_service_sid must look like MG followed by 32 hex characters")
	case cfg.MessagingServiceSID == "" && !phonePattern.MatchString(cfg.From):
		return errors.New("invalid config: set from (E.164) or messaging_service_sid")
	}
	return nil
}

func main() { pluginkit.Main(newPlugin()) }
