package secrets

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// maxDEKRotationFailures caps how many failed services a result lists.
const maxDEKRotationFailures = 50

var (
	// ErrDEKRotationBusy means another rotation is already running.
	ErrDEKRotationBusy = errors.New("secrets: a data key rotation is already running")
	// ErrDEKUnwrap means a stored DEK did not unwrap under the active
	// master key, so nothing for that service was changed.
	ErrDEKUnwrap = errors.New("secrets: data key does not unwrap under the active master key")
	// ErrDEKRotationUnsupported means the store cannot rotate DEKs.
	ErrDEKRotationUnsupported = errors.New("secrets: store does not support data key rotation")
)

// DEKRotationStore is the storage surface per-app DEK rotation needs.
type DEKRotationStore interface {
	ListServiceDEKStatus(ctx context.Context) ([]store.ServiceDEKStatus, error)
	RotateServiceDEK(ctx context.Context, serviceName string, at time.Time,
		rewrite func(wrapped []byte, rows []store.SecretCiphertext) ([]byte, [][]byte, error)) (int, error)
	LatestDEKRotationRun(ctx context.Context) (store.DEKRotationRun, bool, error)
	CreateDEKRotationRun(ctx context.Context, r store.DEKRotationRun) error
	UpdateDEKRotationRun(ctx context.Context, r store.DEKRotationRun) error
}

// DEKRotator rotates per-app data encryption keys: a fresh DEK per app,
// every value re-encrypted under it, committed in one transaction.
type DEKRotator struct {
	m      *Manager
	st     DEKRotationStore
	logger *slog.Logger
	now    func() time.Time
	busy   sync.Mutex
}

// NewDEKRotator builds a rotator over m's active master key and st.
func NewDEKRotator(m *Manager, st DEKRotationStore, logger *slog.Logger) *DEKRotator {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &DEKRotator{m: m, st: st, logger: logger, now: func() time.Time { return time.Now().UTC() }}
}

// DEKRotateResult reports one service rotation.
type DEKRotateResult struct {
	Service   string
	Values    int
	RotatedAt time.Time
}

// DEKRotateFailure names a service that could not be rotated, never why in detail.
type DEKRotateFailure struct {
	Service string
	Reason  string
}

// DEKRotateAllResult reports a rotate-all pass.
type DEKRotateAllResult struct {
	RunID       string
	Resumed     bool
	Total       int
	Rotated     int
	Skipped     int
	FailedCount int
	Failed      []DEKRotateFailure
	// Complete is false when the pass was cancelled before it finished; a
	// rerun resumes it.
	Complete bool
}

// RotateOne rotates serviceName's DEK. A wrong master key, a missing DEK
// or a row changed mid-run leaves every row exactly as it was.
func (r *DEKRotator) RotateOne(ctx context.Context, serviceName string) (DEKRotateResult, error) {
	if !r.busy.TryLock() {
		return DEKRotateResult{}, ErrDEKRotationBusy
	}
	defer r.busy.Unlock()
	return r.rotateService(ctx, serviceName)
}

func (r *DEKRotator) rotateService(ctx context.Context, serviceName string) (DEKRotateResult, error) {
	r.m.valueMu.Lock()
	defer r.m.valueMu.Unlock()
	r.m.mu.RLock()
	defer r.m.mu.RUnlock()
	mk := r.m.mk
	at := r.now()

	n, err := r.st.RotateServiceDEK(ctx, serviceName, at, func(wrapped []byte, rows []store.SecretCiphertext) ([]byte, [][]byte, error) {
		oldDEK, err := mk.UnwrapDEK(WrappedDEK(wrapped))
		if err != nil {
			return nil, nil, ErrDEKUnwrap
		}
		newDEK, newWrapped, err := mk.GenerateDEK()
		if err != nil {
			return nil, nil, fmt.Errorf("secrets: rotate DEK for %q: %w", serviceName, err)
		}
		out := make([][]byte, len(rows))
		for i, row := range rows {
			b := slotBinding(serviceName, row.EnvKey)
			plaintext, _, err := DecryptValue(oldDEK, b, row.Ciphertext)
			if err != nil {
				return nil, nil, fmt.Errorf("secrets: rotate DEK for %q: value %q does not decrypt: %w", serviceName, row.EnvKey, err)
			}
			if out[i], err = EncryptValue(newDEK, b, plaintext); err != nil {
				return nil, nil, fmt.Errorf("secrets: rotate DEK for %q: re-encrypt %q: %w", serviceName, row.EnvKey, err)
			}
		}
		return newWrapped, out, nil
	})
	if err != nil {
		return DEKRotateResult{}, err
	}
	r.logger.Info("secrets: rotated data key", slog.String("service", serviceName), slog.Int("values", n))
	return DEKRotateResult{Service: serviceName, Values: n, RotatedAt: at}, nil
}

// RotateAll rotates every service whose DEK predates the run. A run that
// was cut short (restart, cancelled request) is resumed, not restarted, so
// services already rotated by it are skipped. Finished runs are never reused.
func (r *DEKRotator) RotateAll(ctx context.Context, actor string) (DEKRotateAllResult, error) {
	if !r.busy.TryLock() {
		return DEKRotateAllResult{}, ErrDEKRotationBusy
	}
	defer r.busy.Unlock()

	statuses, err := r.st.ListServiceDEKStatus(ctx)
	if err != nil {
		return DEKRotateAllResult{}, fmt.Errorf("secrets: rotate all: list keys: %w", err)
	}
	run, resumed, err := r.openRun(ctx, actor, len(statuses))
	if err != nil {
		return DEKRotateAllResult{}, err
	}

	res := DEKRotateAllResult{RunID: run.ID, Resumed: resumed, Total: len(statuses)}
	for _, st := range statuses {
		if !st.KeyAgeAnchor().Before(run.StartedAt) {
			res.Skipped++
			continue
		}
		if ctx.Err() != nil {
			break
		}
		if _, err := r.rotateService(ctx, st.ServiceName); err != nil {
			if errors.Is(err, store.ErrServiceDEKNotFound) {
				res.Skipped++
				continue
			}
			if ctx.Err() != nil {
				break
			}
			r.recordFailure(&res, st.ServiceName, err)
			continue
		}
		res.Rotated++
		run.Done = res.Rotated + res.Skipped
		if err := r.st.UpdateDEKRotationRun(ctx, run); err != nil {
			return res, fmt.Errorf("secrets: rotate all: save progress: %w", err)
		}
	}

	res.Complete = ctx.Err() == nil
	run.Done, run.Failed = res.Rotated+res.Skipped, res.FailedCount
	if res.Complete {
		run.FinishedAt = r.now()
	}
	if err := r.st.UpdateDEKRotationRun(context.WithoutCancel(ctx), run); err != nil {
		return res, fmt.Errorf("secrets: rotate all: save result: %w", err)
	}
	return res, nil
}

func (r *DEKRotator) recordFailure(res *DEKRotateAllResult, service string, err error) {
	reason := "rotation failed, the service was left unchanged"
	switch {
	case errors.Is(err, ErrDEKUnwrap):
		reason = ErrDEKUnwrap.Error()
	case errors.Is(err, store.ErrDEKRotationConflict):
		reason = "values changed during rotation, rerun to retry"
	}
	r.logger.Warn("secrets: data key rotation failed", slog.String("service", service), slog.String("error", err.Error()))
	res.FailedCount++
	if len(res.Failed) < maxDEKRotationFailures {
		res.Failed = append(res.Failed, DEKRotateFailure{Service: service, Reason: reason})
	}
}

func (r *DEKRotator) openRun(ctx context.Context, actor string, total int) (store.DEKRotationRun, bool, error) {
	last, ok, err := r.st.LatestDEKRotationRun(ctx)
	if err != nil {
		return store.DEKRotationRun{}, false, fmt.Errorf("secrets: rotate all: load last run: %w", err)
	}
	if ok && !last.Finished() {
		last.Total = total
		return last, true, nil
	}
	run := store.DEKRotationRun{ID: "dek_" + strings.ToLower(rand.Text()[:12]), StartedAt: r.now(), Total: total, Actor: actor}
	if err := r.st.CreateDEKRotationRun(ctx, run); err != nil {
		return store.DEKRotationRun{}, false, fmt.Errorf("secrets: rotate all: %w", err)
	}
	return run, false, nil
}

// ServiceKeyStatus is one app's data key age, never any key bytes.
type ServiceKeyStatus struct {
	Service    string
	CreatedAt  time.Time
	RotatedAt  time.Time
	ValueCount int
	AgeDays    int
}

// DEKStatus is the current key lifecycle picture.
type DEKStatus struct {
	Services []ServiceKeyStatus
	LastRun  *store.DEKRotationRun
}

// Status lists every service's DEK age plus the latest rotate-all run.
func (r *DEKRotator) Status(ctx context.Context) (DEKStatus, error) {
	statuses, err := r.st.ListServiceDEKStatus(ctx)
	if err != nil {
		return DEKStatus{}, fmt.Errorf("secrets: key status: %w", err)
	}
	now := r.now()
	out := DEKStatus{Services: make([]ServiceKeyStatus, 0, len(statuses))}
	for _, st := range statuses {
		out.Services = append(out.Services, ServiceKeyStatus{
			Service: st.ServiceName, CreatedAt: st.CreatedAt, RotatedAt: st.RotatedAt,
			ValueCount: st.ValueCount, AgeDays: int(now.Sub(st.KeyAgeAnchor()).Hours() / 24),
		})
	}
	run, ok, err := r.st.LatestDEKRotationRun(ctx)
	if err != nil {
		return DEKStatus{}, fmt.Errorf("secrets: key status: %w", err)
	}
	if ok {
		out.LastRun = &run
	}
	return out, nil
}
