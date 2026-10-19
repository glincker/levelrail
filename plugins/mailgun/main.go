// Command mailgun emails each alert through the Mailgun HTTP API.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "mailgun"
	pluginVersion = "0.1.0"
	maxText       = 20000
)

var (
	apiBases = map[string]string{"us": "https://api.mailgun.net", "eu": "https://api.eu.mailgun.net"}
	httpc    pluginkit.HTTP

	domainPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
)

type config struct {
	Domain string `json:"domain"`
	Region string `json:"region"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion, Send: send,
		ConfigSchema: `{"type":"object","additionalProperties":false,"required":["domain","from","to"],"properties":{"domain":{"type":"string","description":"Sending domain verified in Mailgun"},"region":{"type":"string","enum":["us","eu"],"description":"Mailgun region, defaults to us"},"from":{"type":"string","description":"From address on the sending domain"},"to":{"type":"string","description":"Recipient address"}}}`,
	}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg config
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	region := strings.ToLower(cfg.Region)
	if region == "" {
		region = "us"
	}
	base, ok := apiBases[region]
	if !ok {
		return pluginkit.Invalid(errors.New("invalid config: region must be us or eu"))
	}
	if !domainPattern.MatchString(cfg.Domain) {
		return pluginkit.Invalid(errors.New("invalid config: domain is missing or malformed"))
	}
	for _, addr := range []string{cfg.From, cfg.To} {
		if _, err := mail.ParseAddress(addr); err != nil {
			return pluginkit.Invalid(errors.New("invalid config: from and to must be email addresses"))
		}
	}
	if strings.TrimSpace(c.Secret) == "" {
		return pluginkit.Invalid(errors.New("the channel target must be the Mailgun API key"))
	}
	payload := url.Values{
		"from":    {cfg.From},
		"to":      {cfg.To},
		"subject": {subject(c.Event)},
		"text":    {pluginkit.Truncate(pluginkit.Text(c.Event), maxText)},
	}.Encode()
	endpoint := base + "/v3/" + url.PathEscape(cfg.Domain) + "/messages"
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth("api", c.Secret)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}, nil, c.Secret)
}

func subject(ev *pluginpb.Event) string {
	s := strings.ReplaceAll(strings.TrimSpace(ev.GetTitle()), "\n", " ")
	if st := strings.TrimSpace(ev.GetStatus()); st != "" {
		s = "[" + strings.ToUpper(st) + "] " + s
	}
	return pluginkit.Truncate(s, 200)
}

func main() { pluginkit.Main(newPlugin()) }
