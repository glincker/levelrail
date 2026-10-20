package api

import (
	"fmt"
	"html"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/mailgroups"
)

const unsubscribePath = "/mail/unsubscribe"

// registerMailPublicRoutes serves the one-click unsubscribe page. It is
// public by design and gated only by the signed token in the link.
func (rt *Router) registerMailPublicRoutes(mux *http.ServeMux) {
	mux.Handle("GET "+unsubscribePath, http.HandlerFunc(rt.handleUnsubscribePage))
	mux.Handle("POST "+unsubscribePath, http.HandlerFunc(rt.handleUnsubscribe))
}

func (rt *Router) unsubscribeEmail(w http.ResponseWriter, r *http.Request) (string, bool) {
	if ok, retry := rt.codeLogin.redeemByIP.allow("unsub|" + clientIP(r)); !ok {
		writeRateLimited(w, retry)
		return "", false
	}
	addr, ok := mailgroups.VerifyUnsubscribe(rt.sec.alertKey, r.URL.Query().Get("t"))
	if !ok {
		writeUnsubscribePage(w, http.StatusBadRequest, rt.brand.Name, "This unsubscribe link is not valid.", "")
		return "", false
	}
	return addr, true
}

func (rt *Router) handleUnsubscribePage(w http.ResponseWriter, r *http.Request) {
	addr, ok := rt.unsubscribeEmail(w, r)
	if !ok {
		return
	}
	form := `<form method="post" action="` + unsubscribePath + `?t=` + html.EscapeString(r.URL.Query().Get("t")) + `"><button type="submit" style="padding:10px 18px;font-size:15px">Unsubscribe</button></form>`
	writeUnsubscribePage(w, http.StatusOK, rt.brand.Name, "Stop announcement emails to "+mailgroups.Mask(addr)+"?", form)
}

// handleUnsubscribe handles the browser form and the RFC 8058 one-click POST.
func (rt *Router) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	addr, ok := rt.unsubscribeEmail(w, r)
	if !ok {
		return
	}
	if rt.mail.db == nil {
		writeUnsubscribePage(w, http.StatusNotImplemented, rt.brand.Name, "Unsubscribing is not available right now.", "")
		return
	}
	if err := rt.mail.db.AddMailSuppression(r.Context(), addr, "unsubscribed from a message"); err != nil {
		rt.logger.Error("api: unsubscribe failed")
		writeUnsubscribePage(w, http.StatusInternalServerError, rt.brand.Name, "Something went wrong. Try again later.", "")
		return
	}
	writeUnsubscribePage(w, http.StatusOK, rt.brand.Name, "You are unsubscribed. Security and account emails still arrive.", "")
}

func writeUnsubscribePage(w http.ResponseWriter, status int, brandName, message, extra string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light dark"><title>%s</title></head><body style="font-family:system-ui,sans-serif;max-width:480px;margin:15vh auto;padding:0 16px"><h1 style="font-size:20px">%s</h1><p>%s</p>%s</body></html>`,
		html.EscapeString(brandName), html.EscapeString(brandName), html.EscapeString(message), extra)
}
