package deploy

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrSuperseded marks a deploy that was not applied because a newer one
// already was. Match it with errors.Is.
var ErrSuperseded = errors.New("deploy superseded")

// SupersededError says why a deploy was not applied.
type SupersededError struct {
	// Reason is store.DeployReasonSuperseded or store.DeployReasonStale.
	Reason  string
	Message string
}

func (e *SupersededError) Error() string { return e.Message }

// Is lets errors.Is(err, ErrSuperseded) match.
func (e *SupersededError) Is(target error) bool { return target == ErrSuperseded }

// SupersededReason returns the recorded reason for a superseded deploy error.
func SupersededReason(err error) (string, bool) {
	var s *SupersededError
	if errors.As(err, &s) {
		return s.Reason, true
	}
	return "", false
}

// CheckOrder rejects an automatic deploy that would overwrite newer desired
// state. Manual deploys and explicit rollbacks always pass.
func CheckOrder(c store.DeployCursor, o store.DeployOrder) error {
	if !o.Automatic {
		return nil
	}
	if o.Sequence > 0 && c.AppliedSequence > o.Sequence {
		return &SupersededError{Reason: store.DeployReasonSuperseded, Message: fmt.Sprintf("superseded: deploy #%d was triggered after this one (#%d) and is already applied", c.AppliedSequence, o.Sequence)}
	}
	if o.CommitSHA == "" || c.CommitSHA == "" || o.CommitSHA == c.CommitSHA {
		return nil
	}
	if c.Before == o.CommitSHA {
		return &SupersededError{Reason: store.DeployReasonStale, Message: fmt.Sprintf("stale commit: %s is the parent of the already deployed %s", short(o.CommitSHA), short(c.CommitSHA))}
	}
	if !o.CommitAt.IsZero() && !c.CommitAt.IsZero() && o.CommitAt.Before(c.CommitAt) {
		return &SupersededError{Reason: store.DeployReasonStale, Message: fmt.Sprintf("stale commit: %s is older than the already deployed %s", short(o.CommitSHA), short(c.CommitSHA))}
	}
	return nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// OrderedStore saves desired state guarded by deploy ordering.
type OrderedStore interface {
	SaveDesiredServiceOrdered(ctx context.Context, svc store.DesiredService, order store.DeployOrder, check store.DeployOrderCheck) error
}

// AttemptRecorder stamps a deploy attempt with the content it resolved to.
type AttemptRecorder interface {
	SetDeployAttemptDigest(ctx context.Context, id, digest, reason string) error
}

// WithOrderedStore enables the stale-deploy guard for requests that carry
// an Order.
func WithOrderedStore(s OrderedStore) Option {
	return func(p *Pipeline) { p.ordered = s }
}

// WithAttemptRecorder records resolved digests on the request's attempt.
func WithAttemptRecorder(r AttemptRecorder) Option {
	return func(p *Pipeline) { p.attempts = r }
}

// saveDesired writes desired through the ordering guard when the request
// carries an Order and the pipeline has an ordered store.
func (p *Pipeline) saveDesired(ctx context.Context, req Request, desired store.DesiredService) error {
	if err := commitPoint(req); err != nil {
		return err
	}
	if req.Order != nil && p.ordered != nil {
		return p.ordered.SaveDesiredServiceOrdered(ctx, desired, *req.Order, CheckOrder)
	}
	return p.store.SaveDesiredService(ctx, desired)
}

func (p *Pipeline) recordDigest(ctx context.Context, req Request, res ImageResolution) {
	if p.attempts == nil || req.AttemptID == "" || (res.Digest == "" && res.Reason == "") {
		return
	}
	if err := p.attempts.SetDeployAttemptDigest(ctx, req.AttemptID, res.Digest, res.Reason); err != nil {
		p.logger.Warn("deploy: record attempt digest failed", "attempt_id", req.AttemptID, "error", err.Error())
	}
}

func commitPoint(req Request) error {
	if req.Commit == nil {
		return nil
	}
	return req.Commit()
}
