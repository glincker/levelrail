package alerting

import (
	"context"
	"strconv"
	"strings"
)

// MaxMessageTemplateLen bounds a channel template so it fits chat message limits.
const MaxMessageTemplateLen = 2000

// MessageTemplateVars lists the {{name}} variables a template may use.
var MessageTemplateVars = []string{"app", "author", "commit", "url", "status", "image", "error", "rule", "resource", "value"}

// RenderMessageTemplate fills {{name}} placeholders; an unknown name is left as typed so a typo shows up in the message.
func RenderMessageTemplate(tpl string, vars map[string]string) string {
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(tpl)
}

func alertTemplateVars(ev Event) map[string]string {
	app := strings.TrimPrefix(ev.Rule.ResourceID, "service:")
	status := "firing"
	if ev.Resolved {
		status = "resolved"
	}
	value := ""
	if ev.Rule.LastValue != nil {
		value = strconv.FormatFloat(*ev.Rule.LastValue, 'f', -1, 64)
	}
	url := ev.LogsLink
	if url == "" {
		url = ev.ChangesLink
	}
	return map[string]string{
		"app": app, "status": status, "rule": ev.Rule.Name, "resource": ev.Rule.ResourceID,
		"value": value, "url": url, "author": "", "commit": "", "image": "", "error": "",
	}
}

func deployTemplateVars(ev DeployOutcome) map[string]string {
	status := "failed"
	if ev.Succeeded {
		status = "succeeded"
	}
	return map[string]string{
		"app": ev.AppName, "status": status, "image": ev.Image, "error": ev.Error,
		"author": ev.Author, "commit": ev.Commit, "url": ev.URL,
		"rule": "", "resource": "service:" + ev.AppName, "value": "",
	}
}

type channelTemplateSource interface {
	ChannelMessageTemplate(ctx context.Context, channelID string) string
}
