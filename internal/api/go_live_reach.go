package api

import (
	"context"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
)

// WithReachabilityOrderer enables the outside port 80 proof, always against
// the staging CA. Without it the ports step keeps the local self-check.
func WithReachabilityOrderer(o certflow.Orderer) Option {
	return func(rt *Router) {
		p, err := certflow.NewProver(o, ACMEStagingDirectoryURL, &rt.certLedger, certflow.LimitsFromEnv(), certflow.ProofTTLFromEnv())
		if err == nil {
			rt.reachProver = p
		}
	}
}

// ProveOutsideReachability asks the staging CA to reach domain on port 80. It
// is the entry point the installer can call; the second result is false when
// no orderer is wired.
func (rt *Router) ProveOutsideReachability(ctx context.Context, domain string) (certflow.Proof, bool) {
	if rt.reachProver == nil {
		return certflow.Proof{}, false
	}
	var errText string
	var errAt time.Time
	if f := acmeFailureFor(domain); f != nil {
		errText, errAt = f.Error, f.At
	}
	return rt.reachProver.Prove(ctx, domain, time.Now(), rt.httpsMaxAttempts(ctx), errText, errAt), true
}

// withOutsideProof replaces the port 80 check with the CA's verdict when it is
// conclusive; open and closed beat the self-check, unknown and limited keep it.
func (rt *Router) withOutsideProof(ctx context.Context, rep domaindoctor.Report, domain string) domaindoctor.Report {
	if !rep.Probed {
		return rep
	}
	proof, ok := rt.ProveOutsideReachability(ctx, domain)
	if !ok || (proof.Outcome != certflow.OutcomeOpen && proof.Outcome != certflow.OutcomeClosed) {
		return rep
	}
	c := domaindoctor.Check{ID: domaindoctor.IDPort80, Title: "Port 80 answers from the internet", State: domaindoctor.StatePass,
		Tier: domaindoctor.TierCertificate, Detail: proof.Detail}
	if proof.Outcome == certflow.OutcomeClosed {
		c.Title, c.State, c.Cause = "Port 80 does not answer from the internet", domaindoctor.StateFail, proof.Detail
		c.Evidence = []string{"CA said: " + proof.CAReason}
	}
	checks := slices.DeleteFunc(slices.Clone(rep.Checks), func(x domaindoctor.Check) bool { return x.ID == domaindoctor.IDPort80 })
	rep.Checks = append(checks, c)
	return rep
}
