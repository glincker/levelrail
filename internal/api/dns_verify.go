package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// dnsVerifyTimeout bounds the provider calls made when a credential is saved.
const dnsVerifyTimeout = 10 * time.Second

// dnsTokenVerification is the additive result of testing a saved credential.
// It carries no credential material and no provider error text.
type dnsTokenVerification struct {
	OK            bool     `json:"ok"`
	MissingScopes []string `json:"missing_scopes"`
	ZonesVisible  int      `json:"zones_visible"`
	Message       string   `json:"message"`
}

// verifyDNSCredential tests a just-saved credential and returns nil when the
// dns.verify_provider_on_save setting is off.
func (rt *Router) verifyDNSCredential(ctx context.Context, name string, build func() (dnszones.Provider, error)) *dnsTokenVerification {
	if !rt.runtimeSettings.Bool(ctx, runtimesettings.KeyDNSVerifyTokenOnSave) {
		return nil
	}
	var p dnszones.Provider
	if rt.dnsVerifyProvider != nil {
		p = rt.dnsVerifyProvider(name)
	}
	if p == nil {
		built, err := build()
		if err != nil {
			return &dnsTokenVerification{MissingScopes: []string{}, Message: "The credential could not be used to build a " + name + " client. Check its format."}
		}
		p = built
	}
	vctx, cancel := context.WithTimeout(ctx, dnsVerifyTimeout)
	defer cancel()
	rep := dnszones.VerifyAccess(vctx, p)
	if rep.Outcome != dnszones.AccessOK {
		rt.logger.Warn("api: dns credential verification failed", slog.String("provider", name), slog.String("outcome", rep.Outcome))
	}
	return verificationFrom(name, rep)
}

func verificationFrom(name string, rep dnszones.AccessReport) *dnsTokenVerification {
	v := &dnsTokenVerification{MissingScopes: []string{}, ZonesVisible: rep.ZonesVisible}
	if rep.Missing != nil {
		v.MissingScopes = rep.Missing
	}
	switch rep.Outcome {
	case dnszones.AccessOK:
		v.OK = true
		v.Message = fmt.Sprintf("%s accepted the credential and %d zone(s) are visible. Edit rights are confirmed on the first change.", name, rep.ZonesVisible)
		if rep.ZonesVisible == 0 {
			v.Message = name + " accepted the credential but it sees no zones yet. Grant it access to the zone you want to manage."
		}
	case dnszones.AccessInvalid:
		v.Message = name + " rejected the credential as invalid or expired. Create a new one and save it again."
	case dnszones.AccessDenied:
		v.Message = name + " accepted the credential but it lacks permissions. Add the listed ones; the product also needs rights to edit records."
	case dnszones.AccessUnreachable:
		v.Message = name + " could not be reached in time. The credential is saved; check the network and save again to re-test."
	default:
		v.Message = name + " returned an unexpected answer. The credential is saved but could not be verified."
	}
	return v
}
