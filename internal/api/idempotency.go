package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	headerIdempotencyKey      = "Idempotency-Key"
	headerIdempotentReplayed  = "Idempotent-Replayed"
	idempotencyKeyMaxLen      = 255
	idempotencyBodyMaxBytes   = 8 << 20
	idempotencyResponseMax    = 1 << 20
	idempotencyRetention      = 24 * time.Hour
	idempotencyConflictDetail = "Idempotency-Key was already used with a different request body"
)

// IdempotencyStore caches responses of retried mutating calls.
type IdempotencyStore interface {
	GetIdempotencyRecord(ctx context.Context, principal, key string) (store.IdempotencyRecord, bool, error)
	PutIdempotencyRecord(ctx context.Context, rec store.IdempotencyRecord) error
	PruneIdempotencyRecords(ctx context.Context, cutoff time.Time) (int64, error)
}

type recordingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
	over   bool
}

func (w *recordingWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(p) > idempotencyResponseMax {
		w.over = true
	} else {
		w.body.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *recordingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// idempotent replays the first response for a repeated Idempotency-Key from the same caller.
// Wrap the inner handler so the caller is already authenticated; requests without the header pass through.
func (rt *Router) idempotent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(headerIdempotencyKey)
		if key == "" || rt.idempotency == nil {
			next(w, r)
			return
		}
		if len(key) > idempotencyKeyMaxLen {
			writeError(w, http.StatusBadRequest, "Idempotency-Key must be at most 255 characters")
			return
		}
		actorType, actorID, _, ok := rt.currentActor(r)
		if !ok {
			next(w, r)
			return
		}
		principal := actorType + ":" + actorID
		body, err := io.ReadAll(io.LimitReader(r.Body, idempotencyBodyMaxBytes+1))
		if err != nil || len(body) > idempotencyBodyMaxBytes {
			next(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+"\n"), body...))
		bodyHash := hex.EncodeToString(sum[:])

		rec, found, err := rt.idempotency.GetIdempotencyRecord(r.Context(), principal, key)
		if err != nil {
			rt.logger.Warn("api: idempotency lookup failed", slog.String("error", err.Error()))
			next(w, r)
			return
		}
		if found && time.Since(rec.CreatedAt) < idempotencyRetention {
			if rec.BodyHash != bodyHash {
				writeError(w, http.StatusUnprocessableEntity, idempotencyConflictDetail)
				return
			}
			if rec.ContentType != "" {
				w.Header().Set("Content-Type", rec.ContentType)
			}
			w.Header().Set(headerIdempotentReplayed, "true")
			w.WriteHeader(rec.Status)
			_, _ = w.Write(rec.Response)
			return
		}

		rw := &recordingWriter{ResponseWriter: w}
		next(rw, r)
		// A 422 is a failed action (deploy, pull) the caller fixes and retries under the same key.
		if rw.status == 0 || rw.over || rw.status >= 500 || rw.status == http.StatusTooManyRequests || rw.status == http.StatusUnprocessableEntity {
			return
		}
		put := store.IdempotencyRecord{
			Principal: principal, Key: key, Route: r.Method + " " + r.URL.Path, BodyHash: bodyHash,
			Status: rw.status, ContentType: w.Header().Get("Content-Type"), Response: rw.body.Bytes(), CreatedAt: time.Now(),
		}
		if err := rt.idempotency.PutIdempotencyRecord(context.WithoutCancel(r.Context()), put); err != nil {
			rt.logger.Warn("api: idempotency store failed", slog.String("error", err.Error()))
		}
	}
}

// pruneIdempotency runs on the audit sweeper tick, so it adds no ticker of its own.
func (rt *Router) pruneIdempotency(ctx context.Context) {
	if rt.idempotency == nil {
		return
	}
	if _, err := rt.idempotency.PruneIdempotencyRecords(ctx, time.Now().Add(-idempotencyRetention)); err != nil {
		rt.logger.Warn("api: idempotency prune failed", slog.String("error", err.Error()))
	}
}
