package golive

import (
	"fmt"
	"time"
)

const certStateIssued = "issued"

func certStep(in Input, dns Step) Step {
	const title = "Certificate is issued automatically"
	if in.Domain == "" {
		return noDomainStep(in, StepCertificate, title)
	}
	c := in.Cert
	st := c.Status
	s := Step{ID: StepCertificate, Title: title, Evidence: []Evidence{}}
	if st.Issuer != "" {
		s.Evidence = append(s.Evidence, ev("Issuer", st.Issuer))
	}
	if !st.NotAfter.IsZero() {
		s.Evidence = append(s.Evidence, ev("Expires", st.NotAfter.UTC().Format("2006-01-02")))
	}
	if !st.RenewalAt.IsZero() {
		s.Evidence = append(s.Evidence, ev("Renews from", st.RenewalAt.UTC().Format("2006-01-02")))
	}
	request := &Action{Kind: ActionRequest, Label: "Get a real certificate", API: c.CertAPI, Body: map[string]any{}}
	switch {
	case st.State == certStateIssued && st.Trusted:
		s.Status, s.Summary = StatusOK, "A trusted certificate is in place and renews on its own."
	case dns.Status != StatusOK:
		s.Status, s.Summary, s.Primary = StatusPending, "Waiting for DNS: Let's Encrypt validates the domain only after it points here.", recheck()
	case st.State == "pending":
		s.Status, s.Summary = StatusPending, "Let's Encrypt is validating the domain over port 80. This usually takes under a minute."
		next := in.Now.Add(in.PollEvery)
		s.NextCheckAt, s.Primary = &next, recheck()
	case st.State == "failed":
		s.Status, s.Summary = StatusFail, orDefault(st.Explanation, "The certificate request failed: "+st.Error)
		if st.Hint != "" {
			s.Evidence = append(s.Evidence, ev("Next step", st.Hint))
		}
		if st.Error != "" {
			s.Evidence = append(s.Evidence, ev("Let's Encrypt said", st.Error))
		}
		s.Primary = retryAction(in, request, st.NextSafeAt)
		s.NextCheckAt = st.NextSafeAt
	case st.State == certStateIssued:
		s.Status, s.Summary = StatusWarn, "The certificate in place is temporary or from a test authority, so browsers will warn visitors."
		s.Primary = request
	case !c.Ready:
		s.Status, s.Summary = StatusFail, orDefault(c.Blocker, "Something blocks the certificate request.")
		if c.BlockerFix != "" {
			s.Evidence = append(s.Evidence, ev("Next step", c.BlockerFix))
		}
		s.Primary, s.NextCheckAt = recheck(), c.NextSafeAt
	default:
		s.Status, s.Summary = StatusPending, "Everything needed is ready. Ask for the certificate."
		s.Primary = request
	}
	return s
}

func retryAction(in Input, request *Action, next *time.Time) *Action {
	if next != nil && next.After(in.Now) {
		r := recheck()
		r.Blocked = fmt.Sprintf("Let's Encrypt asked to wait until %s", next.UTC().Format(time.RFC3339))
		return r
	}
	a := *request
	a.Label = "Try again"
	return &a
}

func liveStep(in Input, prior []Step) Step {
	s := Step{ID: StepLive, Title: "https://your domain serves this app", Evidence: []Evidence{}, Scope: ScopeControlPlane}
	if in.Domain == "" {
		return noDomainStep(in, StepLive, s.Title)
	}
	s.Title = "https://" + in.Domain + " serves this app"
	for _, p := range prior {
		if (p.ID == StepApp || p.ID == StepDNS || p.ID == StepCertificate) && p.Status != StatusOK {
			s.Status, s.Summary, s.Primary = StatusPending, "Waiting for the steps above.", recheck()
			return s
		}
	}
	r := in.Live
	if r == nil {
		s.Status, s.Summary, s.Primary = StatusUnknown, "The end-to-end check has not run yet.", &Action{Kind: ActionRecheck, Label: "Run the check"}
		return s
	}
	s.Evidence = append(s.Evidence,
		ev("Checked from", "this server, so it cannot prove the wider internet can reach it"),
		ev("Proof from outside", "a Let's Encrypt certificate means Let's Encrypt validated the domain from the internet"))
	if r.Status > 0 {
		s.Evidence = append(s.Evidence, ev("HTTP status", fmt.Sprint(r.Status)))
	}
	switch {
	case !r.Reached:
		s.Status, s.Summary = StatusFail, "Could not connect to https://"+in.Domain+" from this server: "+orDefault(r.Error, "no answer")+"."
		s.Primary = recheck()
	case !r.TLSValid:
		s.Status, s.Summary = StatusFail, "The connection works but the certificate is not valid: "+orDefault(r.TLSError, r.Error)+"."
		s.Primary = recheck()
	case r.Match == MatchNone:
		s.Status, s.Summary = StatusWarn, "The address answers over HTTPS but the response does not match this app. Another server or a default page may be answering."
		s.Primary = recheck()
	case r.Match == MatchUnverified:
		s.Status, s.Summary = StatusOK, "https://"+in.Domain+" answers with a valid certificate. The app could not be compared directly, so this is not proof it is this app."
	default:
		s.Status, s.Summary = StatusOK, "https://"+in.Domain+" serves this app with a valid certificate."
		s.Evidence = append(s.Evidence, ev("Matched by", string(r.Match)))
	}
	return s
}
