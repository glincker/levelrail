package secretsource

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Resolver dispatches a Ref to the provider registered under its connection.
// It fails closed: no provider, an unclassified error or an empty value all
// yield an error and never a partial or default value.
type Resolver struct {
	mu        sync.RWMutex
	providers map[string]Provider
	redactor  *Redactor
}

// NewResolver returns an empty Resolver with its own Redactor.
func NewResolver() *Resolver {
	return &Resolver{providers: map[string]Provider{}, redactor: NewRedactor()}
}

// Redactor exposes the masker so callers can scrub their own logs.
func (r *Resolver) Redactor() *Redactor { return r.redactor }

// Register binds a connection name to a provider. Rebinding is rejected;
// call Unregister first so a swap is always deliberate.
func (r *Resolver) Register(conn string, p Provider) error {
	if err := ValidConnName(conn, LimitsFromEnv().MaxConn); err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("%w: nil provider", ErrInvalidRef)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[conn]; ok {
		return fmt.Errorf("secretsource: connection %q already registered", conn)
	}
	r.providers[conn] = p
	return nil
}

// Unregister removes a connection.
func (r *Resolver) Unregister(conn string) {
	r.mu.Lock()
	delete(r.providers, conn)
	r.mu.Unlock()
}

// Provider returns the provider for a connection.
func (r *Resolver) Provider(conn string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[conn]
	return p, ok
}

// Connections lists registered connection names (unordered).
func (r *Resolver) Connections() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	return out
}

// Resolve validates ref, calls the provider and registers the value with the
// redactor. Errors carry the ref (never the value) and one sentinel kind.
func (r *Resolver) Resolve(ctx context.Context, ref Ref) (Value, error) {
	if err := ref.Validate(); err != nil {
		return Value{}, err
	}
	p, ok := r.Provider(ref.Conn)
	if !ok {
		return Value{}, r.fail(ref, ErrUnknownConnection, ErrFailClosed)
	}
	if err := ctx.Err(); err != nil {
		return Value{}, r.fail(ref, ErrUnavailable, err)
	}
	v, err := p.Resolve(ctx, ref)
	if err != nil {
		return Value{}, r.fail(ref, classify(err), err)
	}
	if v.Len() == 0 {
		return Value{}, r.fail(ref, ErrFailClosed, ErrNotFound)
	}
	r.redactor.Add(v.data)
	return v, nil
}

// Renew calls the provider's Renewer, or ErrUnsupported.
func (r *Resolver) Renew(ctx context.Context, conn string, l Lease) (Lease, error) {
	p, ok := r.Provider(conn)
	if !ok {
		return Lease{}, fmt.Errorf("%w: %s", ErrUnknownConnection, conn)
	}
	rn, ok := p.(Renewer)
	if !ok || !p.Capabilities().Renew {
		return Lease{}, fmt.Errorf("%w: renew on %s", ErrUnsupported, conn)
	}
	nl, err := rn.Renew(ctx, l)
	if err != nil {
		return Lease{}, r.redactor.MaskError(fmt.Errorf("%w: renew on %s: %w", classify(err), conn, err))
	}
	return nl, nil
}

// classify keeps a known kind, otherwise treats the failure as unavailable
// (and therefore fail closed) rather than guessing it was a missing secret.
func classify(err error) error {
	for _, k := range []error{ErrNotFound, ErrDenied, ErrUnavailable, ErrFailClosed, ErrUnsupported} {
		if errors.Is(err, k) {
			return k
		}
	}
	return ErrUnavailable
}

func (r *Resolver) fail(ref Ref, kind, cause error) error {
	// Ref is safe to print: it names a location, not a value.
	msg := fmt.Errorf("%w: %s: %w", kind, ref.String(), cause)
	if !errors.Is(kind, ErrFailClosed) && !errors.Is(cause, ErrFailClosed) {
		msg = fmt.Errorf("%w: %w", ErrFailClosed, msg)
	}
	return r.redactor.MaskError(msg)
}
