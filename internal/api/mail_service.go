package api

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsstatus"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailcast"
	"github.com/GLINCKER/levelrail/internal/mailgroups"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Mail tuning, all overridable from the environment.
const (
	envMailDailyCap = "APP_MAIL_BROADCAST_DAILY_CAP"
	envMailDelay    = "APP_MAIL_SEND_DELAY"
	envMailBatch    = "APP_MAIL_SEND_BATCH"

	defaultMailDailyCap = 500
	defaultMailDelay    = 200 * time.Millisecond
	defaultMailBatch    = 50

	mailTickInterval = 15 * time.Second
	mailPageLimit    = 200
)

// MailStore is the persistence behind templates, audiences and broadcasts;
// *store.DB satisfies it, and the routes return 501 without it.
type MailStore interface {
	mailcast.Store
	ListMailTemplates(ctx context.Context) ([]store.MailTemplate, error)
	GetMailTemplate(ctx context.Context, key string) (*store.MailTemplate, error)
	SaveMailTemplate(ctx context.Context, t store.MailTemplate) error
	DeleteMailTemplate(ctx context.Context, key string) error
	ListMailTemplateVersions(ctx context.Context, key string) ([]store.MailTemplateVersion, error)
	GetMailTemplateVersion(ctx context.Context, key string, version int) (*store.MailTemplateVersion, error)
	GetMailLayout(ctx context.Context) (store.MailLayout, error)
	SetMailLayout(ctx context.Context, l store.MailLayout) error
	ListMailGroups(ctx context.Context) ([]store.MailGroup, error)
	GetMailGroup(ctx context.Context, idOrName string) (*store.MailGroup, error)
	SaveMailGroup(ctx context.Context, g store.MailGroup) error
	DeleteMailGroup(ctx context.Context, id string) error
	AddMailSuppression(ctx context.Context, email, reason string) error
	RemoveMailSuppression(ctx context.Context, email string) error
	CreateMailBroadcast(ctx context.Context, b store.MailBroadcast, recipients []store.MailRecipient) error
	ListMailBroadcasts(ctx context.Context, limit int) ([]store.MailBroadcast, error)
	CountMailBroadcastRecipientsSince(ctx context.Context, since time.Time) (int, error)
	ListUsers(ctx context.Context) ([]store.User, error)
	UserTeamRoles(ctx context.Context, userID string) (map[string]string, error)
	ListProjects(ctx context.Context) ([]store.Project, error)
	ListOrganizations(ctx context.Context) ([]store.Organization, error)
}

type mailState struct {
	db     MailStore
	runner *mailcast.Runner
	// boxes holds mailbox ownership; engine is nil in production (the default factory is used).
	boxes  MailboxStore
	engine mailEngineFactory
	// demo is nil unless dev mode and APP_MAIL_DEMO are both on (mail_demo.go).
	demo   *mailDemo
	backup *mailBackupConfig
	// sysEmail guards the system email actions (mail_system_email.go).
	sysEmail sysEmailRuntime
	// nsResolver is nil in production (net.DefaultResolver); nsCache keeps detections briefly.
	nsResolver mailrecords.NSResolver
	nsCache    mailNSCache
	// statusResolver is nil in production (net.DefaultResolver); statusCache absorbs retry bursts.
	statusResolver dnsstatus.Resolver
	statusCache    dnsstatus.Cache
}

func (rt *Router) attachMail(s any) {
	rt.initMailDemo()
	if mb, ok := s.(MailboxStore); ok {
		rt.mail.boxes = mb
	}
	ms, ok := s.(MailStore)
	if !ok {
		return
	}
	rt.mail.db = ms
	rt.mail.runner = mailcast.New(ms, rt.renderBroadcastMessage, rt.sendBroadcastMessage, rt.logger)
	rt.mail.runner.Delay = envDuration(envMailDelay, defaultMailDelay)
	rt.mail.runner.Batch = envInt(envMailBatch, defaultMailBatch)
}

// RunMailQueue starts scheduled broadcasts when due and resumes interrupted
// ones, until ctx ends. A no-op without a mail store.
func (rt *Router) RunMailQueue(ctx context.Context) {
	if rt.mail.runner == nil {
		return
	}
	t := time.NewTicker(mailTickInterval)
	defer t.Stop()
	for {
		rt.mail.runner.Tick(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (rt *Router) mailBranding(ctx context.Context) mailtemplates.Branding {
	base := rt.emailLinkBase(ctx)
	instance := rt.brand.Name
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		instance = u.Host
	}
	b := mailtemplates.Branding{
		Name: rt.brand.Name, PrimaryColor: rt.brand.PrimaryColor, PrimaryColorDark: rt.brand.PrimaryColorDark,
		SupportEmail: rt.brand.SupportEmail, DashboardURL: base, InstanceName: instance,
	}
	return withBrandColor(b, rt.emailBranding(ctx).BrandColor)
}

// withBrandColor applies the operator's accent color to both color schemes.
func withBrandColor(b mailtemplates.Branding, color string) mailtemplates.Branding {
	if color != "" {
		b.PrimaryColor, b.PrimaryColorDark = color, color
	}
	return b
}

// mailLayout is the legacy mail_layout row overlaid with the sender-identity branding, which wins where set.
func (rt *Router) mailLayout(ctx context.Context) mailtemplates.Layout {
	var layout mailtemplates.Layout
	if rt.mail.db != nil {
		if l, err := rt.mail.db.GetMailLayout(ctx); err != nil {
			rt.logger.Warn("api: load mail layout failed", slog.String("error", err.Error()))
		} else {
			layout = mailtemplates.Layout{HeaderImageURL: l.HeaderImageURL, FooterText: l.FooterText}
		}
	}
	return overlayBranding(layout, rt.emailBranding(ctx))
}

func overlayBranding(layout mailtemplates.Layout, b mailtemplates.EmailBranding) mailtemplates.Layout {
	if b.LogoURL != "" {
		layout.HeaderImageURL = b.LogoURL
	}
	if b.FooterText != "" {
		layout.FooterText = b.FooterText
	}
	layout.FooterAddress, layout.FooterLinks, layout.ShowPoweredBy = b.FooterAddress, b.FooterLinks, b.ShowPoweredBy
	return layout
}

// effectiveTemplate is the operator's override for key or the built-in default.
func (rt *Router) effectiveTemplate(ctx context.Context, key string) (mailtemplates.Template, bool) {
	ev, ok := mailtemplates.LookupEvent(key)
	if !ok {
		return mailtemplates.Template{}, false
	}
	if rt.mail.db != nil {
		if t, err := rt.mail.db.GetMailTemplate(ctx, key); err == nil {
			cand := mailtemplates.Template{Subject: t.Subject, Preheader: t.Preheader, Body: t.Body}
			if mailtemplates.ValidateTemplate(ev, cand) == nil {
				return cand, true
			}
		}
	}
	return ev.Default, true
}

// recipientValues fills the recipient-scope tokens, guessing a first name from the address when none is known.
func recipientValues(addr, displayName, role string) map[string]string {
	if displayName == "" {
		displayName, _, _ = strings.Cut(addr, "@")
	}
	first, _, _ := strings.Cut(strings.TrimSpace(displayName), " ")
	if first == "" {
		first = "there"
	}
	return map[string]string{"email": addr, "display_name": displayName, "first_name": first, "role": role}
}

// mailMessage renders event key for one recipient inside the shared layout.
func (rt *Router) mailMessage(ctx context.Context, key, to string, values map[string]string, opts mailtemplates.Options) (email.Message, bool) {
	tpl, ok := rt.effectiveTemplate(ctx, key)
	if !ok {
		return email.Message{}, false
	}
	vals := recipientValues(to, "", "")
	for k, v := range values {
		vals[k] = v
	}
	out := mailtemplates.Render(rt.mailBranding(ctx), rt.mailLayout(ctx), tpl, vals, opts)
	return email.Message{To: to, Subject: out.Subject, Text: out.Text, HTML: out.HTML}, true
}

// sendMailEvent renders and sends one platform email through the configured sender.
func (rt *Router) sendMailEvent(ctx context.Context, key, to string, values map[string]string) error {
	if rt.emailSender == nil {
		return email.ErrNotConfigured
	}
	m, ok := rt.mailMessage(ctx, key, to, values, mailtemplates.Options{})
	if !ok {
		return email.ErrNotConfigured
	}
	return email.SendMessage(ctx, rt.emailSender, m)
}

func (rt *Router) mailProviderConfigured(ctx context.Context) bool {
	if rt.emailSender == nil {
		return false
	}
	if c, ok := rt.emailSender.(interface{ Configured(context.Context) bool }); ok {
		return c.Configured(ctx)
	}
	return true
}

func (rt *Router) unsubscribeURL(ctx context.Context, addr string) string {
	return rt.emailLinkBase(ctx) + "/mail/unsubscribe?t=" + mailgroups.SignUnsubscribe(rt.sec.alertKey, addr)
}

func (rt *Router) renderBroadcastMessage(ctx context.Context, b store.MailBroadcast, rc store.MailRecipient) (email.Message, error) {
	display, role := "", ""
	if rc.UserID != "" {
		if u, err := rt.auth.GetUserByID(ctx, rc.UserID); err == nil {
			display = u.DisplayName
		}
	}
	vals := recipientValues(rc.Email, display, role)
	unsub := rt.unsubscribeURL(ctx, rc.Email)
	out := mailtemplates.Render(rt.mailBranding(ctx), rt.mailLayout(ctx), mailtemplates.Template{Subject: b.Subject, Body: b.Body}, vals, mailtemplates.Options{UnsubscribeURL: unsub})
	return email.Message{
		To: rc.Email, Subject: out.Subject, Text: out.Text, HTML: out.HTML,
		Headers: map[string]string{"List-Unsubscribe": "<" + unsub + ">", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"},
	}, nil
}

func (rt *Router) sendBroadcastMessage(ctx context.Context, m email.Message) error {
	if rt.emailSender == nil {
		return email.ErrNotConfigured
	}
	return email.SendMessage(ctx, rt.emailSender, m)
}

// mailDirectory snapshots users, their roles and project ownership for audience rules.
func (rt *Router) mailDirectory(ctx context.Context) (mailgroups.Directory, error) {
	users, err := rt.mail.db.ListUsers(ctx)
	if err != nil {
		return mailgroups.Directory{}, err
	}
	projects, err := rt.mail.db.ListProjects(ctx)
	if err != nil {
		return mailgroups.Directory{}, err
	}
	d := mailgroups.Directory{ProjectTeams: map[string]string{}}
	for _, p := range projects {
		d.ProjectTeams[p.ID] = p.OrgID
	}
	for _, u := range users {
		roles, err := rt.mail.db.UserTeamRoles(ctx, u.ID)
		if err != nil {
			return mailgroups.Directory{}, err
		}
		mu := mailgroups.User{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Role: mailRole(u, roles), LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt}
		for team := range roles {
			mu.TeamIDs = append(mu.TeamIDs, team)
		}
		d.Users = append(d.Users, mu)
	}
	return d, nil
}

// mailRole is the user's highest team role, or one derived from abilities when they have no team.
func mailRole(u store.User, teamRoles map[string]string) string {
	rank := map[string]int{"owner": 4, "admin": 3, "operator": 2, "viewer": 1}
	best := ""
	for _, r := range teamRoles {
		if r == store.TeamRoleDeveloper {
			r = "operator"
		}
		if rank[r] > rank[best] {
			best = r
		}
	}
	if best != "" {
		return best
	}
	switch {
	case hasAbility(u.Abilities, AbilityRoot):
		return "owner"
	case hasAbility(u.Abilities, AbilityDeploy):
		return "admin"
	case hasAbility(u.Abilities, AbilityWrite):
		return "operator"
	}
	return "viewer"
}

func (rt *Router) mailSuppressedSet(ctx context.Context) (map[string]bool, error) {
	list, err := rt.mail.db.ListMailSuppressions(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s.Email] = true
	}
	return out, nil
}

func firstWord(name string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	return first
}
