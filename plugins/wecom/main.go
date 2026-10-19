// Command wecom delivers alerts to a WeCom (WeChat Work) group through a group robot webhook.
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
	pluginName    = "wecom"
	pluginVersion = "0.1.0"
	maxText       = 2000
)

var (
	webhookHost = "qyapi.weixin.qq.com"
	allowHTTP   bool
	httpc       pluginkit.HTTP
)

func newPlugin() pluginkit.Plugin {
	return pluginkit.Plugin{Name: pluginName, Version: pluginVersion, Send: send}
}

func send(ctx context.Context, c pluginkit.Call) pluginkit.Result {
	var cfg struct{}
	if err := pluginkit.ParseConfig(c.Config, &cfg); err != nil {
		return pluginkit.Invalid(err)
	}
	u, err := url.Parse(c.Secret)
	if err != nil || u.Hostname() != webhookHost || u.Path != "/cgi-bin/webhook/send" || u.Query().Get("key") == "" || (u.Scheme != "https" && !allowHTTP) {
		return pluginkit.Invalid(errors.New("the channel target must be a WeCom group robot webhook URL (https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...)"))
	}
	body, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": pluginkit.Truncate(pluginkit.Text(c.Event), maxText)},
	})
	return httpc.Deliver(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		return req, nil
	}, accept, c.Secret, u.Query().Get("key"))
}

// WeCom answers HTTP 200 with a non-zero errcode for a rejected message.
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
