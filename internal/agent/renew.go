package agent

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// Defaults for RenewConfig. Renewing at two thirds of the lifetime leaves
// the last third to retry through outages (the step-ca and cert-manager
// default).
const (
	DefaultRenewFraction = 2.0 / 3.0
	defaultRenewJitter   = 0.05
	defaultRenewRetryMin = time.Minute
	defaultRenewRetryMax = time.Hour
)

// RenewConfig tunes when and how often the agent renews its certificate.
type RenewConfig struct {
	// Fraction of the certificate's lifetime after which renewal starts.
	Fraction float64
	// Jitter is the fraction of lifetime added at random so nodes enrolled
	// together do not all renew at once.
	Jitter             float64
	RetryMin, RetryMax time.Duration
}

func (c RenewConfig) withDefaults() RenewConfig {
	if c.Fraction <= 0 || c.Fraction >= 1 {
		c.Fraction = DefaultRenewFraction
	}
	if c.Jitter < 0 || c.Jitter >= 1-c.Fraction {
		c.Jitter = 0
	} else if c.Jitter == 0 {
		c.Jitter = min(defaultRenewJitter, (1-c.Fraction)/2)
	}
	if c.RetryMin <= 0 {
		c.RetryMin = defaultRenewRetryMin
	}
	if c.RetryMax < c.RetryMin {
		c.RetryMax = max(defaultRenewRetryMax, c.RetryMin)
	}
	return c
}

// IdentityPersister stages a renewed identity durably and then confirms or
// rolls it back. *IdentityFile implements it.
type IdentityPersister interface {
	Stage(next *Identity) error
	Confirm() error
	Rollback() error
}

// IdentityChecker proves an identity is accepted by the control plane
// without opening a Session.
type IdentityChecker func(ctx context.Context, id *Identity) error

// Renewer keeps an agent's certificate fresh over its authenticated
// connection (ADR 021).
type Renewer struct {
	holder  *IdentityHolder
	persist IdentityPersister
	check   IdentityChecker
	cfg     RenewConfig
	logger  *slog.Logger

	now   func() time.Time
	after func(time.Duration) <-chan time.Time
	// jit overrides the per-certificate jitter, for tests.
	jit func() float64

	trigger chan struct{}
	updates chan struct{}
	mu      sync.Mutex
	st      RenewalStatus
}

// RenewalStatus is the renewer's schedule and last failure, reported to the
// control plane so the node page can show it.
type RenewalStatus struct {
	NextRenewAt         time.Time
	LastError           string
	LastFailedAt        time.Time
	NextRetryAt         time.Time
	ConsecutiveFailures int
}

// NewRenewer builds a Renewer. check defaults to CheckIdentityAt(addr)
// when nil.
func NewRenewer(holder *IdentityHolder, persist IdentityPersister, check IdentityChecker, cfg RenewConfig, logger *slog.Logger) *Renewer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Renewer{
		holder: holder, persist: persist, check: check, cfg: cfg.withDefaults(), logger: logger,
		now: time.Now, after: time.After,
		trigger: make(chan struct{}, 1), updates: make(chan struct{}, 1),
	}
}

// Status returns the current schedule and failure state.
func (r *Renewer) Status() RenewalStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

// Updates signals (coalesced) that Status changed.
func (r *Renewer) Updates() <-chan struct{} { return r.updates }

// RenewNow makes Run attempt a renewal immediately, skipping any wait.
func (r *Renewer) RenewNow() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

func (r *Renewer) setStatus(f func(*RenewalStatus)) {
	r.mu.Lock()
	f(&r.st)
	r.mu.Unlock()
	select {
	case r.updates <- struct{}{}:
	default:
	}
}

// jitterFor is stable per certificate, so a restart recomputes the same
// renewal time from the certificate on disk instead of re-rolling it.
func (r *Renewer) jitterFor(leaf *x509.Certificate) float64 {
	if r.jit != nil {
		return r.jit()
	}
	sum := sha256.Sum256(leaf.SerialNumber.Bytes())
	return float64(binary.BigEndian.Uint64(sum[:8])>>11) / float64(1<<53)
}

// Holder returns the identity holder the renewer updates.
func (r *Renewer) Holder() *IdentityHolder { return r.holder }

// RenewAt is when id should be renewed: Fraction of its lifetime plus up
// to Jitter more.
func (r *Renewer) RenewAt(id *Identity) (time.Time, error) {
	leaf, err := id.Leaf()
	if err != nil {
		return time.Time{}, err
	}
	life := leaf.NotAfter.Sub(leaf.NotBefore)
	offset := time.Duration(float64(life) * (r.cfg.Fraction + r.cfg.Jitter*r.jitterFor(leaf)))
	return leaf.NotBefore.Add(offset), nil
}

// Run renews whenever the current certificate reaches its renewal time,
// retrying with capped backoff, until ctx is done. A RenewNow call skips
// the wait. The schedule comes from the certificate, so a restart resumes it.
func (r *Renewer) Run(ctx context.Context, client agentpb.AgentServiceClient) {
	retry := r.cfg.RetryMin
	justRenewed := false
	for {
		wait := r.cfg.RetryMin
		if at, err := r.RenewAt(r.holder.Current()); err == nil {
			wait = at.Sub(r.now())
			r.setStatus(func(s *RenewalStatus) { s.NextRenewAt = at; s.NextRetryAt = time.Time{} })
		}
		// A certificate issued already past its renewal point (tiny lifetime,
		// skewed clock) must not turn into a renewal loop against the server.
		if justRenewed {
			wait = max(wait, r.cfg.RetryMin)
		}
		if wait > 0 && !r.sleep(ctx, wait) {
			return
		}
		err := r.RenewOnce(ctx, client)
		if err == nil {
			retry, justRenewed = r.cfg.RetryMin, true
			continue
		}
		justRenewed = false
		if ctx.Err() != nil {
			return
		}
		r.logRenewFailure(err)
		r.setStatus(func(s *RenewalStatus) {
			s.LastError, s.LastFailedAt = err.Error(), r.now()
			s.NextRetryAt = r.now().Add(retry)
			s.ConsecutiveFailures++
		})
		if !r.sleep(ctx, retry) {
			return
		}
		retry = min(retry*2, r.cfg.RetryMax)
	}
}

// sleep waits d, returning early with true on RenewNow and false when ctx
// is done.
func (r *Renewer) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-r.trigger:
		return true
	case <-r.after(d):
		return true
	}
}

func (r *Renewer) logRenewFailure(err error) {
	id := r.holder.Current()
	attrs := []any{slog.String("node_id", id.NodeID), slog.String("error", err.Error())}
	if leaf, lerr := id.Leaf(); lerr == nil {
		attrs = append(attrs, slog.Time("not_after", leaf.NotAfter))
	}
	if isAuthRejection(err) {
		r.logger.Error("agent: certificate renewal rejected: re-enroll this node from the control plane", attrs...)
		return
	}
	r.logger.Warn("agent: certificate renewal failed, will retry", attrs...)
}

// RenewOnce keeps the current certificate on failure, which the control plane still accepts during the overlap window.
func (r *Renewer) RenewOnce(ctx context.Context, client agentpb.AgentServiceClient) error {
	cur := r.holder.Current()
	keyPEM, csrDER, err := NewKeyAndCSR(cur.NodeID)
	if err != nil {
		return err
	}
	resp, err := client.Renew(ctx, &agentpb.RenewRequest{CsrDer: csrDER})
	if err != nil {
		return fmt.Errorf("agent: renew: %w", err)
	}
	caPEM := resp.GetCaCertPem()
	if len(caPEM) == 0 {
		caPEM = cur.CACertPEM
	}
	if err := validateIssued(cur.NodeID, resp.GetClientCertPem(), keyPEM, caPEM, r.now()); err != nil {
		return err
	}
	next := &Identity{NodeID: cur.NodeID, ClientCertPEM: resp.GetClientCertPem(), ClientKeyPEM: keyPEM, CACertPEM: caPEM}
	if err := r.persist.Stage(next); err != nil {
		return fmt.Errorf("agent: renew: persist: %w", err)
	}
	if err := r.check(ctx, next); err != nil {
		if rbErr := r.persist.Rollback(); rbErr != nil {
			return errors.Join(fmt.Errorf("agent: renew: new certificate failed its test connection: %w", err), rbErr)
		}
		return fmt.Errorf("agent: renew: new certificate failed its test connection, kept the old one: %w", err)
	}
	r.holder.Set(next)
	r.setStatus(func(s *RenewalStatus) {
		s.LastError, s.LastFailedAt, s.NextRetryAt, s.ConsecutiveFailures = "", time.Time{}, time.Time{}, 0
	})
	if err := r.persist.Confirm(); err != nil {
		r.logger.Warn("agent: renew: drop identity backup failed", slog.String("error", err.Error()))
	}
	r.logger.Info("agent: certificate renewed", slog.String("node_id", next.NodeID), slog.Time("not_after", resp.GetNotAfter().AsTime()))
	return nil
}

// CheckIdentityAt returns an IdentityChecker that dials addr presenting
// the identity and calls CheckIdentity, requiring it to be current.
func CheckIdentityAt(addr string) IdentityChecker {
	return func(ctx context.Context, id *Identity) error {
		conn, err := dialWithIdentity(addr, id)
		if err != nil {
			return err
		}
		defer func() { _ = conn.Close() }()
		resp, err := agentpb.NewAgentServiceClient(conn).CheckIdentity(ctx, &agentpb.CheckIdentityRequest{})
		if err != nil {
			return fmt.Errorf("agent: check identity: %w", err)
		}
		if resp.GetNodeId() != id.NodeID || !resp.GetCurrent() {
			return status.Errorf(codes.Unauthenticated, "control plane does not hold this certificate as current for node %q", id.NodeID)
		}
		return nil
	}
}

func dialWithIdentity(addr string, id *Identity) (*grpc.ClientConn, error) {
	cert, err := tls.X509KeyPair(id.ClientCertPEM, id.ClientKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("agent: parse identity certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(id.CACertPEM) {
		return nil, fmt.Errorf("agent: parse CA certificate: no valid certificate found")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert}, RootCAs: pool, NextProtos: []string{"h2"},
	})))
	if err != nil {
		return nil, fmt.Errorf("agent: dial %q: %w", addr, err)
	}
	return conn, nil
}

// isAuthRejection reports whether err is the control plane refusing this
// identity, as opposed to it being unreachable.
func isAuthRejection(err error) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() == codes.Unauthenticated || s.Code() == codes.PermissionDenied
}

// IsAuthRejection is isAuthRejection for callers outside this package.
func IsAuthRejection(err error) bool { return isAuthRejection(err) }
