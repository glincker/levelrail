// Command zulip posts alerts to a Zulip stream topic as a bot.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "zulip"
	pluginVersion = "0.1.0"
	maxText       = 9000
)

var httpc pluginkit.HTTP

type config struct {
	BaseURL   string `json:"base_url"`
	Stream    string `json:"stream"`
	Topic     string `json:"topic"`
	AllowHTTP bool   `json:"allow_http"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion, Send: send,
		ConfigSchema: `{"type":"object","additionalProperties":false,"required":["base_url","stream"],"properties":{"base_url":{"type":"string","description":"Zulip server, e.g. https://zulip.example.com"},"stream":{"type":"string"},"topic":{"type":"string","description":"Defaults to alerts"},"allow_http":{"type":"boolean","description":"Permit plain http for a lab server"}}}`,
	}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg config
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	base, err := pluginkit.BaseURL(cfg.BaseURL, cfg.AllowHTTP)
	if err != nil {
		return pluginkit.Invalid(err)
	}
	if strings.TrimSpace(cfg.Stream) == "" {
		return pluginkit.Invalid(errors.New("invalid config: stream is required"))
	}
	botEmail, apiKey, ok := strings.Cut(c.Secret, ":")
	if !ok || botEmail == "" || apiKey == "" {
		return pluginkit.Invalid(errors.New("the channel target must be <bot email>:<api key>"))
	}
	topic := cfg.Topic
	if topic == "" {
		topic = "alerts"
	}
	form := url.Values{
		"type":    {"stream"},
		"to":      {cfg.Stream},
		"topic":   {pluginkit.Truncate(topic, 60)},
		"content": {pluginkit.Truncate(pluginkit.Text(c.Event), maxText)},
	}.Encode()
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base.String()+"/api/v1/messages", strings.NewReader(form))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(botEmail, apiKey)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}, nil, c.Secret, apiKey)
}

func main() { pluginkit.Main(newPlugin()) }
