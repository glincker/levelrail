package domaindoctor

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

// Canonical report order; stages below decide what runs concurrently.
const (
	slotResolves = iota
	slotPointsHere
	slotProxied
	slotAAAA
	slotWildcard
	slotApexCNAME
	slotCAA
	slotApp
	slotHeldBack
	slotPortOwners
	slotPort80
	slotTLS
	slotHTTP
	slotRedirects
	slotHSTS
	slotStoredCert
	slotCount
)

// IDTimeout is reported when the run budget ran out before every check ran.
const IDTimeout = "doctor.timeout"

// Run executes every check and never returns an error: a check that cannot
// run reports unavailable or skipped instead.
func Run(ctx context.Context, facts Facts, probes Probes, opts Options) Report {
	return RunStream(ctx, facts, probes, opts, nil)
}

// RunStream is Run that also calls emit with each check as soon as it
// finishes, so a caller can show first results before the slow probes end.
// emit calls are serialized but arrive in completion order; the returned
// report is in canonical order.
func RunStream(ctx context.Context, facts Facts, probes Probes, opts Options, emit func(Check)) Report {
	opts = opts.withDefaults()
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	facts.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(facts.Domain), "."))
	rep := &Report{Domain: facts.Domain, App: facts.App, CheckedAt: opts.Now().UTC()}
	rn := &run{facts: facts, probes: probes, opts: opts, report: rep}

	var (
		mu       sync.Mutex
		emitMu   sync.Mutex
		results  [slotCount]Check
		done     [slotCount]bool
		timedOut atomic.Bool
	)
	do := func(slot int, fn func(context.Context) Check) {
		if ctx.Err() != nil {
			timedOut.Store(true)
			return
		}
		c := fn(ctx)
		if c.ID == "" {
			return
		}
		rn.finishFix(&c)
		mu.Lock()
		results[slot], done[slot] = c, true
		mu.Unlock()
		if emit != nil {
			emitMu.Lock()
			emit(c)
			emitMu.Unlock()
		}
	}

	do(slotResolves, rn.checkResolves)
	rn.decideProbe()
	parallel(
		func() { do(slotPointsHere, rn.checkPointsHere) },
		func() { do(slotProxied, rn.checkProxied) },
		func() { do(slotAAAA, rn.checkAAAA) },
		func() { do(slotWildcard, rn.checkWildcard) },
		func() { do(slotApexCNAME, rn.checkApexCNAME) },
		func() { do(slotCAA, rn.checkCAA) },
		func() { do(slotApp, rn.checkApp) },
		func() { do(slotHeldBack, rn.checkHeldBack) },
		func() { do(slotPortOwners, rn.checkPortOwners) },
		func() { do(slotStoredCert, rn.checkStoredCert) },
		func() { do(slotPort80, rn.checkPort80) },
		func() {
			do(slotTLS, rn.checkTLS)
			parallel(
				func() { do(slotHTTP, rn.checkHTTPStatus) },
				func() { do(slotRedirects, rn.checkRedirects) },
			)
			do(slotHSTS, rn.checkHSTS)
		},
	)

	for i := range results {
		if done[i] {
			rep.Checks = append(rep.Checks, results[i])
		}
	}
	if timedOut.Load() {
		c := Check{ID: IDTimeout, Title: "Doctor finished every check", State: StateUnavailable, Tier: TierHygiene,
			Detail: "the doctor ran out of time before every check finished; run it again"}
		rep.Checks = append(rep.Checks, c)
		if emit != nil {
			emit(c)
		}
	}
	rep.Status = overall(rep.Checks)
	rep.Records = PlanRecords(facts.Domain, facts.ServerIPs, facts.ServerHost)
	rep.Propagation = rn.propagation(rep.Checks)
	return *rep
}

func parallel(fns ...func()) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}
	wg.Wait()
}

const (
	minNextCheck     = 15 * time.Second
	maxNextCheck     = 5 * time.Minute
	waitingNextCheck = 30 * time.Second
)

// propagation reads the DNS checks: settled needs nothing, propagating waits
// out the longest TTL a resolver still caches, waiting polls a missing record.
func (rn *run) propagation(checks []Check) *Propagation {
	var resolves *Check
	for i := range checks {
		if checks[i].ID == IDDNSResolves {
			resolves = &checks[i]
		}
	}
	if resolves == nil {
		return nil
	}
	p := &Propagation{State: PropagationSettled, Detail: "every resolver agrees"}
	var next time.Duration
	switch resolves.State {
	case StateFail:
		p.State = PropagationWaiting
		p.Detail = "no resolver knows this name yet; new records usually appear within a few minutes"
		next = waitingNextCheck
	case StateWarn:
		p.State = PropagationPropagating
		p.Detail = "resolvers disagree while the old answer's TTL runs out"
		next = clampNextCheck(time.Duration(rn.maxTTL()) * time.Second)
	default:
		return p
	}
	p.NextCheckSeconds = int(next.Seconds())
	p.NextCheckAt = rn.opts.Now().UTC().Add(next)
	return p
}

func (rn *run) maxTTL() uint32 {
	var m uint32
	for _, a := range rn.answers {
		m = max(m, a.TTL)
	}
	return m
}

func clampNextCheck(d time.Duration) time.Duration {
	return min(max(d, minNextCheck), maxNextCheck)
}

// finishFix sets the fix kind and the secondary CLI hint.
func (rn *run) finishFix(c *Check) {
	f := c.Fix
	if f == nil {
		return
	}
	switch {
	case f.Action != nil && f.Action.Kind == ActionRequest && isWriteRequest(f.Action.API):
		f.Kind = fixit.KindAuto
		if rn.facts.CLIName != "" && f.CLI == "" {
			f.CLI = rn.facts.CLIName + " domains diagnose " + rn.facts.Domain + " --fix"
		}
	case len(f.Records) > 0 || f.Action != nil:
		f.Kind = fixit.KindGuided
	default:
		f.Kind = fixit.KindManual
	}
}

func isWriteRequest(api string) bool {
	method, _, _ := strings.Cut(api, " ")
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}
