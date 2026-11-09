package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Alert channel import. No type here has a token or webhook field: the notify URL goes from
// the source straight into the channel create call and is never stored, printed or journaled.
const (
	channelUnsupported = "unsupported"
	journalKindChannel = "alert_channel"
)

// dokployChannelKinds maps source provider types onto channel kinds that exist here.
var dokployChannelKinds = map[string]string{
	"slack": "slack", "discord": "discord", "telegram": "telegram", "email": "email", "gotify": "gotify",
	"ntfy": "ntfy", "lark": "lark", "pushover": "pushover", "custom": "generic",
}

type channelImport struct {
	Channels    []importedChannel    `json:"channels"`
	Unsupported []unsupportedChannel `json:"unsupported,omitempty"`
	// useSecrets and target are set only for --apply with --include-secret-values.
	useSecrets bool
	target     func(ctx context.Context, sourceID string) (platformimport.NotifyTarget, error)
	checkURL   func(string) error
}

type importedChannel struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Server     string `json:"server,omitempty"`
	// Enabled is false unless --enable-imported-channels was passed.
	Enabled    bool   `json:"enabled"`
	ID         string `json:"id,omitempty"`
	State      string `json:"state"`
	Detail     string `json:"detail,omitempty"`
	sourceID   string
	sourceKind string
	recipient  string
	hasSecret  bool
	hasAuth    bool
}

type unsupportedChannel struct {
	SourceName string `json:"source_name"`
	SourceKind string `json:"source_kind"`
	Reason     string `json:"reason"`
}

type channelPlanOptions struct {
	enable bool
	limit  int
	// checkURL is the import network policy check; tests swap it to avoid DNS.
	checkURL func(string) error
}

func channelPlanOptionsFromEnv(ctx context.Context, enable bool, lookupEnv func(string) (string, bool)) channelPlanOptions {
	policy := platformimport.NetworkPolicy{AllowPrivate: envIsTrue(lookupEnv, platformimport.AllowPrivateEnv), AllowLoopback: envIsTrue(lookupEnv, platformimport.AllowLoopbackEnv)}
	return channelPlanOptions{enable: enable,
		limit:    envPositiveInt(lookupEnv, envMigrateBackupLimit, defaultMigrateBackupLimit, ceilingMigrateBackupLimit),
		checkURL: func(raw string) error { return platformimport.ValidateNotifyEndpoint(ctx, raw, policy) }}
}

// serverProblem checks a self-hosted provider base URL, which is printed in hints and plans.
func serverProblem(raw string, check func(string) error) string {
	u, err := url.Parse(raw)
	switch {
	case raw == "" || err != nil:
		return "the provider has no usable server URL"
	case hasControlChar(raw):
		return "the server URL holds control characters"
	case u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return "the server URL carries credentials or a query; create this channel by hand"
	}
	if check != nil {
		if err := check(raw); err != nil {
			return "server URL refused by the import network policy: " + cleanDetail(err.Error())
		}
	}
	return ""
}

func planChannel(p platformimport.NotificationProvider, o channelPlanOptions, budget int, used map[string]bool) importedChannel {
	c := importedChannel{SourceName: cleanDetail(p.Name), Kind: dokployChannelKinds[p.Kind], Enabled: o.enable, sourceID: p.SourceID, sourceKind: p.Kind,
		recipient: p.Recipient, hasSecret: p.HasSecret, hasAuth: p.HasAccessToken, State: backupNeedsSecret}
	if c.Kind == "email" {
		c.State = backupWouldCreate
	}
	name, nameOK := importName(p.Name)
	c.Name = name
	server := ""
	if p.Kind == "gotify" || p.Kind == "ntfy" {
		server = serverProblem(p.Server, o.checkURL)
	}
	switch {
	case budget <= 0:
		c.State, c.Detail = backupSkipped, "over the per-run import limit ("+envMigrateBackupLimit+")"
	case !nameOK:
		c.State, c.Detail = backupReview, "the source name is blank, symbols only or over 63 characters; create this channel by hand"
	case used[c.Name]:
		c.State, c.Detail = backupSkipped, "another source provider maps to the same name; create this one by hand"
	case p.Mismatch:
		c.State, c.Detail = backupReview, "the provider settings do not match its kind; create this channel by hand"
	case p.HasHeaders:
		c.State, c.Detail = backupReview, "the custom webhook sends extra headers, which a generic channel does not carry; create it by hand"
	case c.Kind == "email" && (p.Recipient == "" || hasControlChar(p.Recipient) || strings.ContainsAny(p.Recipient, " ,;")):
		c.State, c.Detail = backupReview, "the email provider has no usable recipient address"
	case server != "":
		c.State, c.Detail = backupReview, server
	default:
		if p.Kind == "gotify" || p.Kind == "ntfy" {
			c.Server = p.Server
		}
		if p.ExtraRecipients > 0 {
			c.Detail = fmt.Sprintf("only the first recipient is imported; add a channel for each of the other %d", p.ExtraRecipients)
		}
	}
	return c
}

// planDokployChannels is pure apart from the server URL check: names, kinds and hosts only.
func planDokployChannels(disc *platformimport.Discovery, o channelPlanOptions) *channelImport {
	if len(disc.Notifications) == 0 {
		return nil
	}
	imp := &channelImport{checkURL: o.checkURL}
	used := map[string]bool{}
	budget := o.limit
	for _, p := range disc.Notifications {
		if _, ok := dokployChannelKinds[p.Kind]; !ok {
			imp.Unsupported = append(imp.Unsupported, unsupportedChannel{SourceName: cleanDetail(p.Name), SourceKind: cleanDetail(p.Kind),
				Reason: "no matching channel kind here; create a generic webhook channel or another kind by hand"})
			continue
		}
		c := planChannel(p, o, budget, used)
		if c.Name != "" {
			used[c.Name] = true
		}
		budget--
		imp.Channels = append(imp.Channels, c)
	}
	return imp
}

// channelHint is the manual create command. It carries placeholders only, never a token or address.
func channelHint(c *importedChannel) string {
	cmd := "token not imported; run: channels create --name " + shQuote(c.Name) + " --kind " + shQuote(c.Kind)
	switch c.Kind {
	case "pushover":
		cmd += " --pushover-user-key '<user-key>' --pushover-api-token '<api-token>'"
	case "telegram":
		cmd += " --notify-url 'https://api.telegram.org/bot<bot-token>/sendMessage?chat_id=<chat-id>'"
	case "gotify":
		cmd += " --notify-url " + shQuote(cleanDetail(c.Server)+"/message?token=<app-token>")
	case "ntfy":
		u := cleanDetail(c.Server) + "/<topic>"
		if c.hasAuth {
			u += "?auth=<access-token>"
		}
		cmd += " --notify-url " + shQuote(u)
	case "email":
		cmd += " --notify-url '<address>'"
	default:
		cmd += " --notify-url '<" + c.Kind + "-webhook-url>'"
	}
	if !c.Enabled {
		cmd += " --disabled"
	}
	return cmd
}

// finishPlan marks which channels an apply would create and prints the manual command for the rest.
func (imp *channelImport) finishPlan(useSecrets bool) {
	if imp == nil {
		return
	}
	imp.useSecrets = useSecrets
	for i := range imp.Channels {
		c := &imp.Channels[i]
		if c.State != backupNeedsSecret {
			continue
		}
		if useSecrets && c.hasSecret {
			c.State = backupWouldCreate
			continue
		}
		c.Detail = channelHint(c)
	}
}

func channelFingerprint(ch apiclient.NotificationChannelResource) string {
	sum := sha256.Sum256([]byte(ch.Kind + "\n" + ch.Name + "\n" + ch.ID + "\n" + ch.CreatedAt + "\n" + ch.UpdatedAt))
	return journalFingerprintPfx + hex.EncodeToString(sum[:])
}

// redactNotify removes the notify URL and its path and query parts from a server message.
func redactNotify(msg, notify string) string {
	if notify == "" {
		return cleanDetail(msg)
	}
	parts := []string{notify, url.QueryEscape(notify)}
	if u, err := url.Parse(notify); err == nil {
		parts = append(parts, u.RawQuery, u.EscapedPath(), u.Path)
		parts = append(parts, strings.Split(u.Path, "/")...)
		for _, vs := range u.Query() {
			parts = append(parts, vs...)
		}
	}
	for _, p := range parts {
		if len(p) >= 4 {
			msg = strings.ReplaceAll(msg, p, "<redacted>")
		}
	}
	return cleanDetail(msg)
}

func pendingChannel(state string) bool {
	return state == backupNeedsSecret || state == backupWouldCreate
}

// applyChannelImport creates through the same API call as channels create, so scoping,
// URL validation, secret storage and audit match a manual create.
func applyChannelImport(ctx context.Context, client *Client, imp *channelImport, jr *applyJournal) {
	if imp == nil {
		return
	}
	existing, err := client.ListNotificationChannels(ctx)
	if err != nil {
		for i := range imp.Channels {
			if pendingChannel(imp.Channels[i].State) {
				imp.Channels[i].State, imp.Channels[i].Detail = backupFailed, "could not list channels: "+cleanDetail(err.Error())
			}
		}
		return
	}
	for i := range imp.Channels {
		applyChannel(ctx, client, imp, &imp.Channels[i], existing, jr)
	}
}

// resumeChannel reuses a channel only when its recorded id still lists with the recorded fingerprint.
func resumeChannel(c *importedChannel, r *journalResource, existing []apiclient.NotificationChannelResource) {
	for _, ch := range existing {
		if ch.ID == r.TargetID && r.Fingerprint != "" && channelFingerprint(ch) == r.Fingerprint {
			c.ID, c.State, c.Detail = ch.ID, backupCreated, "created by this run earlier"
			return
		}
	}
	c.State, c.Detail = backupReview, "cannot tell whether the channel this run created still exists unchanged; not created again"
}

func applyChannel(ctx context.Context, client *Client, imp *channelImport, c *importedChannel, existing []apiclient.NotificationChannelResource, jr *applyJournal) {
	if !pendingChannel(c.State) {
		return
	}
	if r := jr.findCreated(journalKindChannel, c.Name); r != nil {
		resumeChannel(c, r, existing)
		return
	}
	for _, ch := range existing {
		if ch.Name == c.Name {
			c.State, c.Detail = backupExists, "warning: a channel with this name already exists and was left unchanged"
			return
		}
	}
	notify := c.recipient
	if c.Kind != "email" {
		if !imp.useSecrets || !c.hasSecret || imp.target == nil {
			c.State, c.Detail = backupNeedsSecret, channelHint(c)
			return
		}
		tgt, err := imp.target(ctx, c.sourceID)
		if err != nil || tgt.URL == "" {
			c.State, c.Detail = backupNeedsSecret, "the source returned no usable token; "+channelHint(c)
			return
		}
		if tgt.Kind != c.sourceKind || tgt.Server != c.Server {
			c.State, c.Detail = backupReview, "source changed since the plan; run the import again"
			return
		}
		notify = tgt.URL
		if imp.checkURL != nil {
			if err := imp.checkURL(notify); err != nil {
				c.State, c.Detail = backupReview, "refused by the import network policy: "+cleanDetail(err.Error())
				return
			}
		}
	}
	enabled := c.Enabled
	created, err := client.CreateNotificationChannel(ctx, createNotificationChannelRequest{Name: c.Name, Kind: c.Kind, NotifyURL: notify, Enabled: &enabled})
	if err != nil {
		c.State, c.Detail = backupFailed, redactNotify(err.Error(), notify)
		jr.recordBackup(journalKindChannel, c.Name, stateFailed, "", "", "")
		return
	}
	if created.Enabled && !enabled {
		unwantedEnabled(ctx, client, c, created, jr)
		return
	}
	c.ID, c.State, c.Detail = created.ID, backupCreated, ""
	jr.recordBackup(journalKindChannel, c.Name, stateDone, created.ID, "", channelFingerprint(created))
}

// unwantedEnabled deletes a channel the server created enabled against the request, so an
// import never starts sending. If the delete fails, the journal keeps it for undo.
func unwantedEnabled(ctx context.Context, client *Client, c *importedChannel, created apiclient.NotificationChannelResource, jr *applyJournal) {
	if err := client.DeleteNotificationChannel(ctx, created.ID); err != nil {
		c.ID, c.State = created.ID, backupFailed
		c.Detail = "the server created it enabled although disabled was requested and it could not be deleted; delete it with channels delete " + shQuote(created.ID) + ": " + cleanDetail(err.Error())
		if r := jr.recordBackup(journalKindChannel, c.Name, stateDone, created.ID, "", channelFingerprint(created)); r != nil {
			jr.record(r, stepPaused, stateFailed)
		}
		return
	}
	c.State, c.Detail = backupFailed, "the server created it enabled although disabled was requested, so it was deleted"
	jr.recordBackup(journalKindChannel, c.Name, stateFailed, "", "", "")
}

func printChannelImport(out io.Writer, imp *channelImport) {
	if imp == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "\nAlert channels: %d provider(s), %d unsupported\n", len(imp.Channels), len(imp.Unsupported))
	for _, c := range imp.Channels {
		mode := "disabled"
		if c.Enabled {
			mode = "enabled"
		}
		where := ""
		if c.Server != "" {
			where = ", " + cleanDetail(c.Server)
		}
		_, _ = fmt.Fprintf(out, "  - channel %s (from %q, %s%s, %s): %s\n", c.Name, c.SourceName, c.Kind, where, mode, c.State)
		if c.Detail != "" {
			_, _ = fmt.Fprintf(out, "      %s\n", c.Detail)
		}
	}
	for _, u := range imp.Unsupported {
		_, _ = fmt.Fprintf(out, "  - provider %q (%s): unsupported, %s\n", u.SourceName, u.SourceKind, u.Reason)
	}
}

// planChannelUndo removes a channel only when its recorded id still lists with the recorded
// fingerprint. An empty or partial list may mean a restricted token, so that is "cannot tell".
func planChannelUndo(ctx context.Context, client *Client, r *journalResource) (action, reason string) {
	if r.TargetID == "" || r.Fingerprint == "" {
		return undoSkip, "the journal holds no id and fingerprint for it, never removed"
	}
	list, err := client.ListNotificationChannels(ctx)
	switch {
	case err != nil:
		return undoSkip, "could not list channels: " + cleanDetail(err.Error())
	case len(list) == 0:
		return undoSkip, "no channels are visible to this token, cannot tell whether it is gone; nothing removed"
	}
	for _, ch := range list {
		if ch.ID != r.TargetID {
			continue
		}
		if channelFingerprint(ch) != r.Fingerprint {
			return undoSkip, "changed since the run, never removed"
		}
		return undoRemove, ""
	}
	return undoSkip, "not visible to this token by its recorded id, cannot tell whether it is gone; nothing removed"
}
