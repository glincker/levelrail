package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/mail/demo"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
)

// mailDemo is the dev-only fake mail world. rt.mail.demo stays nil unless
// dev mode and APP_MAIL_DEMO are both on, and every seam checks that first.
type mailDemo struct {
	mu        sync.Mutex
	engine    *demo.Engine
	zone      *demo.Zone
	portsOpen bool
}

// initMailDemo is the single place demo mode is decided.
func (rt *Router) initMailDemo() {
	on, ignored := demo.EnabledFromEnv(devModeEnabled())
	if ignored {
		rt.logger.Warn("APP_MAIL_DEMO is ignored because APP_DEV_MODE is not enabled")
	}
	if !on {
		return
	}
	rt.logger.Warn("mail demo mode is on: the Mail area serves fake data and writes nothing to a real DNS provider or mail server")
	rt.mail.demo = &mailDemo{engine: demo.NewEngine(), zone: demo.NewZone()}
}

func (d *mailDemo) engineFactory(context.Context, string) (mail.Engine, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.engine, nil
}

// mailZone is the zone seam: the fake zone in demo mode, the real provider otherwise.
func (rt *Router) mailZone(w http.ResponseWriter, r *http.Request) (dnszones.Provider, dnszones.Zone, bool) {
	if d := rt.mail.demo; d != nil {
		return d.zone, d.zone.ZoneFor(r.PathValue("zone")), true
	}
	return rt.requireZone(w, r)
}

// mailDemoFillSpec supplies the values a real install would read from the host.
func (rt *Router) mailDemoFillSpec(s *mailrecords.Spec) {
	if rt.mail.demo == nil {
		return
	}
	if s.Domain == "" {
		s.Domain = demo.DefaultDomain
	}
	if s.MailHost == "" {
		s.MailHost = "mail." + s.Domain
	}
	if s.IPv4 == "" && s.IPv6 == "" {
		s.IPv4 = demo.MailIPv4
	}
	if s.DKIMSelector == "" {
		s.DKIMSelector = demo.DKIMSelector
	}
	if s.DKIMPublicKey == "" {
		s.DKIMPublicKey = demo.DKIMPublicKey()
	}
}

func (rt *Router) mailDemoCheckArgs(domain string, exp *mailrecords.Expect) string {
	if rt.mail.demo == nil {
		return domain
	}
	if domain == "" {
		domain = demo.DefaultDomain
	}
	if exp.MailHost == "" {
		exp.MailHost = "mail." + domain
	}
	if exp.DKIMSelector == "" {
		exp.DKIMSelector = demo.DKIMSelector
	}
	return domain
}

func (rt *Router) mailDemoStatus(w http.ResponseWriter, r *http.Request) bool {
	if rt.mail.demo == nil {
		return false
	}
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		domain = demo.DefaultDomain
	}
	writeJSON(w, http.StatusOK, mailStatusResponse{
		App: demo.AppName, Installed: true, ContainerState: "running", AdminReachable: true, TLS: "self-signed",
		Domain: domain, Hostname: "mail." + domain, Demo: true,
		NextStep: "DEMO DATA: nothing here is real. Plan and apply DNS to see the checklist turn green, then reset the demo zone to start over.",
	})
	return true
}

func (rt *Router) mailDemoPorts(w http.ResponseWriter) bool {
	d := rt.mail.demo
	if d == nil {
		return false
	}
	d.mu.Lock()
	open := d.portsOpen
	d.mu.Unlock()
	bind, fw := bindaddr.Private, "blocked"
	if open {
		bind, fw = bindaddr.Public, "allowed"
	}
	out := mailPortsResponse{App: demo.AppName, Installed: true, WebListener: "private", Port25Note: mailPort25Note,
		Warning: mailOpenWarn, BindScopeNote: mailBindNote, NextStep: "DEMO DATA: opening the ports only flips a fake flag.", Ports: []mailPortInfo{}}
	for _, p := range mailOpenPorts {
		p.Published, p.Bind, p.Firewall = true, bind, fw
		out.Ports = append(out.Ports, p)
	}
	writeJSON(w, http.StatusOK, out)
	return true
}

func (rt *Router) mailDemoPortsOpen(w http.ResponseWriter) bool {
	d := rt.mail.demo
	if d == nil {
		return false
	}
	d.mu.Lock()
	was := d.portsOpen
	d.portsOpen = true
	d.mu.Unlock()
	out := mailPortsOpenResponse{App: demo.AppName, Changed: []int{}, Warning: mailOpenWarn, Result: "unchanged",
		Message: "DEMO DATA: the ports were already marked open in the fake state."}
	if !was {
		for _, p := range mailOpenPorts {
			out.Changed = append(out.Changed, p.Port)
		}
		out.Result, out.Message = "opened", "DEMO DATA: only the fake port state changed; no firewall or container was touched."
	}
	writeJSON(w, http.StatusOK, out)
	return true
}

// handleMailDemoReset handles POST /api/v1/mail/demo/reset: back to the seeded fixtures.
func (rt *Router) handleMailDemoReset(w http.ResponseWriter, r *http.Request) {
	d := rt.mail.demo
	if d == nil {
		writeError(w, http.StatusNotFound, "mail demo mode is not enabled")
		return
	}
	d.zone.Reset()
	d.mu.Lock()
	d.engine, d.portsOpen = demo.NewEngine(), false
	d.mu.Unlock()
	rt.auditDNS(r, "mail_demo.reset", d.zone.ZoneFor(demo.DefaultDomain), "demo state reset", http.StatusOK)
	rt.logger.Info("mail demo state reset", slog.String("by", "api"))
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}
