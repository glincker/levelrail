// Command dingtalk delivers alerts to a DingTalk group through a custom robot webhook.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

const (
	pluginName    = "dingtalk"
	pluginVersion = "0.1.0"
	maxText       = 4000
)

var (
	webhookHost = "oapi.dingtalk.com"
	allowHTTP   bool
	httpc       pluginkit.HTTP
)

type config struct {
	Keyword string `json:"keyword"`
}

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{
		Name: pluginName, Version: pluginVersion, Send: send,
		ConfigSchema: `{"type":"object","additionalProperties":false,"properties":{"keyword":{"type":"string","description":"Custom keyword the robot requires in every message"}}}`,
	}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg config
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	u, err := url.Parse(c.Secret)
	if err != nil || u.Hostname() != webhookHost || u.Path != "/robot/send" || u.Query().Get("access_token") == "" || (u.Scheme != "https" && !allowHTTP) {
		return pluginkit.Invalid(errors.New("the channel target must be a DingTalk robot webhook URL (https://oapi.dingtalk.com/robot/send?access_token=...)"))
	}
	text := pluginkit.Text(c.Event)
	if cfg.Keyword != "" {
		text = cfg.Keyword + " " + text
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": pluginkit.Truncate(text, maxText)},
	})
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		return req, nil
	}, accept, c.Secret, u.Query().Get("access_token"))
}

// DingTalk answers HTTP 200 with a non-zero errcode for a rejected message.
func accept(body []byte) (bool, string) {
	var r struct {
		Code int    `json:"errcode"`
		Msg  string `json:"errmsg"`
	}
	if json.Unmarshal(body, &r) != nil {
		return false, "unreadable response from the service"
	}
	if r.Code != 0 {
		return false, fmt.Sprintf("rejected (errcode %d): %s", r.Code, pluginkit.Truncate(r.Msg, 200))
	}
	return true, ""
}

func main() { pluginkit.Main(newPlugin()) }
