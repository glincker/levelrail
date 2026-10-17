package alerting

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
)

// NotifyPluginPrefix starts the channel kind of a notification_channel plugin,
// for example "plugin:example-notify".
const NotifyPluginPrefix = "plugin:"

// Plugin channel failures. A plugin kind never falls back to the generic webhook shape.
var (
	ErrPluginUnavailable        = errors.New("alerting: plugin channels are not available on this control plane")
	ErrPluginChannelUnsupported = errors.New("alerting: plugin channels are not supported for this notice yet")
)

// IsPluginKind reports whether k names a plugin channel.
func IsPluginKind(k NotifyKind) bool { return strings.HasPrefix(string(k), NotifyPluginPrefix) }

// PluginName returns the plugin a plugin kind points at.
func PluginName(k NotifyKind) string { return strings.TrimPrefix(string(k), NotifyPluginPrefix) }

// PluginNotice is the event handed to a plugin: only what it needs to deliver one message.
type PluginNotice struct {
	ID         string
	Title      string
	Body       string
	Status     string
	Severity   string
	ResourceID string
	At         time.Time
	// LogLines are log excerpts the plugin runtime releases only to plugins
	// approved for read:sensitive.
	LogLines []string
}

// PluginSender delivers a notice through an installed plugin. secret is the
// channel's stored credential, passed for this call only.
type PluginSender interface {
	SendPlugin(ctx context.Context, plugin, secret string, n PluginNotice) error
}

// NewNotifierWithPlugins is NewNotifier plus plugin channel support. plugins may be nil.
func NewNotifierWithPlugins(client *http.Client, sender email.Sender, pushSender PushSender, plugins PluginSender, r Rule) Notifier {
	if IsPluginKind(r.NotifyKind) {
		return pluginNotifier{plugins: plugins, plugin: PluginName(r.NotifyKind), secret: r.NotifyURL}
	}
	return NewNotifier(client, sender, pushSender, r)
}

type pluginNotifier struct {
	plugins PluginSender
	plugin  string
	secret  string
}

func (n pluginNotifier) Notify(ctx context.Context, ev Event) error {
	if n.plugins == nil {
		return ErrPluginUnavailable
	}
	status := "firing"
	if ev.Resolved {
		ev.LogLines, status = nil, "resolved"
	}
	lines := ev.LogLines
	ev.LogLines = nil
	notice := PluginNotice{
		ID: ev.Rule.ID, Title: fmt.Sprintf("%s %s", strings.ToUpper(status), ev.Rule.Name), Body: summaryBody(ev),
		Status: status, Severity: ev.Rule.Severity, ResourceID: ev.Rule.ResourceID, At: time.Now().UTC(), LogLines: lines,
	}
	return n.plugins.SendPlugin(ctx, n.plugin, n.secret, notice)
}
