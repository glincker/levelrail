package dataexport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type worker struct {
	cancel    context.CancelFunc
	updatedAt string
	dv        *deliverer
	done      chan struct{}
}

// Run reconciles the running workers against the stored destinations until
// ctx ends. A destination that fails to start is retried every resync.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.d.Config.Resync)
	defer ticker.Stop()
	s.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-ticker.C:
		case <-s.nudge:
		}
		s.reconcile(ctx)
	}
}

func (s *Service) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, w := range s.workers {
		w.cancel()
		<-w.done
		delete(s.workers, id)
	}
}

func (s *Service) reconcile(ctx context.Context) {
	dests, err := s.d.Store.ListExportDestinations(ctx)
	if err != nil {
		s.d.Logger.Error("dataexport: list destinations failed", slog.String("error", err.Error()))
		return
	}
	want := map[string]store.ExportDestination{}
	for _, d := range dests {
		if d.Enabled {
			want[d.ID] = d
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, w := range s.workers {
		if d, ok := want[id]; !ok || d.UpdatedAt != w.updatedAt {
			w.cancel()
			<-w.done
			delete(s.workers, id)
		}
	}
	for id, d := range want {
		if _, ok := s.workers[id]; ok {
			continue
		}
		w, err := s.startWorker(ctx, d)
		if err != nil {
			if s.startErrs[id] != err.Error() {
				s.startErrs[id] = err.Error()
				s.d.Logger.Error("dataexport: start destination failed",
					slog.String("destination_id", id), slog.String("kind", string(d.Kind)), slog.String("error", err.Error()))
				s.recordStartFailure(ctx, d, err)
			}
			continue
		}
		delete(s.startErrs, id)
		s.workers[id] = w
	}
	for id := range s.startErrs {
		if _, ok := want[id]; !ok {
			delete(s.startErrs, id)
		}
	}
}

func (s *Service) recordStartFailure(ctx context.Context, d store.ExportDestination, cause error) {
	st, err := s.d.Store.GetExportStatus(ctx, d.ID)
	if err != nil {
		return
	}
	st.LastError = truncate(cause.Error(), 300)
	st.LastErrorAt = s.d.Now().UTC().Format(time.RFC3339Nano)
	_ = s.d.Store.SaveExportStatus(ctx, st)
}

// sender is the kind-specific delivery surface built from a destination.
type sender struct {
	logs    func(context.Context, []LogRecord) error
	audit   auditSender
	metrics func(context.Context, []byte) error
	close   func()
}

func (s *Service) build(ctx context.Context, d store.ExportDestination) (sender, error) {
	blob, err := s.readSecrets(ctx, d)
	if err != nil {
		return sender{}, err
	}
	switch d.Kind {
	case store.ExportKindLogsHTTP:
		h := &httpLogSender{url: d.Endpoint, client: s.client, headers: blob.Headers}
		return sender{logs: h.send}, nil
	case store.ExportKindLogsLoki:
		l := &lokiSender{url: LokiPushURL(d.Endpoint), client: s.client, headers: blob.Headers, static: d.Options.Labels, job: s.prefix[:len(s.prefix)-1]}
		return sender{logs: l.send}, nil
	case store.ExportKindLogsSyslog:
		sl, err := newSyslogSender(d.Endpoint, s.prefix[:len(s.prefix)-1])
		if err != nil {
			return sender{}, err
		}
		return sender{logs: sl.send, close: sl.close}, nil
	case store.ExportKindAuditWebhook:
		return sender{audit: &webhookAuditSender{url: d.Endpoint, client: s.client, headers: blob.Headers, hmacKey: blob.HMACKey, prefix: s.hdrPfx, now: s.d.Now}}, nil
	case store.ExportKindAuditS3:
		if s.d.Objects == nil {
			return sender{}, errors.New("object storage is not configured on this control plane")
		}
		prefix := d.Options.Prefix
		if prefix == "" {
			prefix = "audit-log"
		}
		return sender{audit: &s3AuditSender{put: s.d.Objects, targetID: d.Options.StorageTargetID, prefix: prefix}}, nil
	case store.ExportKindMetricsOTLP:
		url := OTLPURL(d.Endpoint)
		return sender{metrics: func(ctx context.Context, body []byte) error {
			return postBody(ctx, s.client, url, "application/json", blob.Headers, body, "otlp metrics")
		}}, nil
	}
	return sender{}, fmt.Errorf("unknown destination kind %q", d.Kind)
}

func (s *Service) startWorker(parent context.Context, d store.ExportDestination) (*worker, error) {
	snd, err := s.build(parent, d)
	if err != nil {
		return nil, err
	}
	if isLogKind(d.Kind) && s.d.Logs == nil {
		return nil, errors.New("log stream is not available on this control plane")
	}
	ctx, cancel := context.WithCancel(parent)
	dv := newDeliverer(s.d.Config)
	prev, _ := s.d.Store.GetExportStatus(ctx, d.ID)
	dv.st.sent.Store(prev.SentTotal)
	dv.st.dropped.Store(prev.DroppedTotal)
	dv.st.failures.Store(prev.FailuresTotal)
	dv.st.setCursor(prev.CursorAt, prev.CursorID)

	w := &worker{cancel: cancel, updatedAt: d.UpdatedAt, dv: dv, done: make(chan struct{})}
	var wg sync.WaitGroup
	spawn := func(fn func()) {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	switch {
	case snd.logs != nil:
		s.startLogs(ctx, d, snd, dv, spawn)
	case snd.audit != nil:
		spawn(func() { s.runAudit(ctx, d, snd.audit, dv) })
	case snd.metrics != nil:
		spawn(func() { s.runMetricsPush(ctx, d, snd.metrics, dv) })
	}
	spawn(func() { s.persistLoop(ctx, d.ID, dv) })
	go func() {
		wg.Wait()
		if snd.close != nil {
			snd.close()
		}
		close(w.done)
	}()
	return w, nil
}

func (s *Service) startLogs(ctx context.Context, d store.ExportDestination, snd sender, dv *deliverer, spawn func(func())) {
	q := newQueue[LogRecord](s.d.Config.QueueLines)
	dv.st.depth = q.len
	apps := map[string]struct{}{}
	for _, a := range d.Apps {
		apps[a] = struct{}{}
	}
	ch, unsubscribe := s.d.Logs.SubscribeAll()
	spawn(func() {
		defer unsubscribe()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				r := NewLogRecord(e)
				if r.matches(apps, d.MinLevel) {
					if n := q.push(r); n > 0 {
						dv.st.dropped.Add(int64(n))
					}
				}
			}
		}
	})
	b := &batcher[LogRecord]{q: q, d: dv, send: snd.logs, size: LogRecord.size}
	spawn(func() { b.run(ctx) })
}

func (s *Service) runAudit(ctx context.Context, d store.ExportDestination, snd auditSender, dv *deliverer) {
	snap := dv.st.snapshot()
	at, id := snap.CursorAt, snap.CursorID
	if at == "" && !d.Options.Backfill {
		at = store.FormatAuditTime(s.d.Now())
		dv.st.setCursor(at, id)
		s.saveStatus(ctx, d.ID, dv)
	}
	ticker := time.NewTicker(s.d.Config.AuditInterval)
	defer ticker.Stop()
	for {
		for {
			rows, err := s.d.Store.ListAuditEntriesAfter(ctx, at, id, nil, s.d.Config.AuditBatch)
			if err != nil {
				s.d.Logger.Error("dataexport: read audit log failed", slog.String("destination_id", d.ID), slog.String("error", err.Error()))
				break
			}
			if len(rows) == 0 {
				break
			}
			batch, err := newAuditBatch(rows)
			if err != nil {
				s.d.Logger.Error("dataexport: encode audit batch failed", slog.String("destination_id", d.ID), slog.String("error", err.Error()))
				break
			}
			if err := dv.do(ctx, len(rows), func(c context.Context) error { return snd.sendAudit(c, batch) }); err != nil {
				break
			}
			at, id = batch.lastAt, batch.lastID
			dv.st.setCursor(at, id)
			s.saveStatus(ctx, d.ID, dv)
			if len(rows) < s.d.Config.AuditBatch {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) runMetricsPush(ctx context.Context, d store.ExportDestination, post func(context.Context, []byte) error, dv *deliverer) {
	interval := s.d.Config.MetricsInterval
	if d.Options.IntervalSeconds > 0 {
		interval = time.Duration(d.Options.IntervalSeconds) * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		series, err := s.Series(ctx, nil)
		if err != nil {
			s.d.Logger.Warn("dataexport: collect metrics failed", slog.String("destination_id", d.ID), slog.String("error", err.Error()))
		}
		if len(series) == 0 {
			continue
		}
		body, err := EncodeOTLP(series, s.prefix[:len(s.prefix)-1], s.d.Now())
		if err != nil {
			continue
		}
		_ = dv.do(ctx, len(series), func(c context.Context) error { return post(c, body) })
	}
}

func (s *Service) persistLoop(ctx context.Context, id string, dv *deliverer) {
	ticker := time.NewTicker(s.d.Config.StatusInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			s.saveStatus(flush, id, dv)
			cancel()
			return
		case <-ticker.C:
			s.saveStatus(ctx, id, dv)
		}
	}
}

func (s *Service) saveStatus(ctx context.Context, id string, dv *deliverer) {
	snap := dv.st.snapshot()
	st := store.ExportStatus{
		DestinationID: id, SentTotal: snap.Sent, DroppedTotal: snap.Dropped, FailuresTotal: snap.Failures,
		QueueDepth: snap.QueueDepth, CircuitOpen: snap.CircuitOpen, CursorAt: snap.CursorAt, CursorID: snap.CursorID,
		LastError: snap.LastError,
	}
	if !snap.LastSuccess.IsZero() {
		st.LastSuccessAt = snap.LastSuccess.UTC().Format(time.RFC3339Nano)
	}
	if !snap.LastErrorAt.IsZero() {
		st.LastErrorAt = snap.LastErrorAt.UTC().Format(time.RFC3339Nano)
	}
	if err := s.d.Store.SaveExportStatus(ctx, st); err != nil && ctx.Err() == nil {
		s.d.Logger.Warn("dataexport: persist status failed", slog.String("destination_id", id), slog.String("error", err.Error()))
	}
}

// Status returns the live delivery state of a destination, falling back to
// the persisted one when no worker is running.
func (s *Service) Status(ctx context.Context, id string) (store.ExportStatus, error) {
	s.mu.Lock()
	w, ok := s.workers[id]
	s.mu.Unlock()
	persisted, err := s.d.Store.GetExportStatus(ctx, id)
	if err != nil {
		return store.ExportStatus{}, fmt.Errorf("dataexport: %w", err)
	}
	if !ok {
		return persisted, nil
	}
	snap := w.dv.st.snapshot()
	out := persisted
	out.SentTotal, out.DroppedTotal, out.FailuresTotal = snap.Sent, snap.Dropped, snap.Failures
	out.QueueDepth, out.CircuitOpen = snap.QueueDepth, snap.CircuitOpen
	out.CursorAt, out.CursorID = snap.CursorAt, snap.CursorID
	if !snap.LastSuccess.IsZero() {
		out.LastSuccessAt = snap.LastSuccess.UTC().Format(time.RFC3339Nano)
	}
	if snap.LastError != "" || snap.LastSuccess.IsZero() {
		out.LastError = snap.LastError
		if !snap.LastErrorAt.IsZero() {
			out.LastErrorAt = snap.LastErrorAt.UTC().Format(time.RFC3339Nano)
		}
	}
	return out, nil
}

// Test sends one probe through the destination without touching its queue,
// counters or cursor, and returns the delivery error if any.
func (s *Service) Test(ctx context.Context, id string) error {
	d, err := s.d.Store.GetExportDestination(ctx, id)
	if err != nil {
		return fmt.Errorf("dataexport: %w", err)
	}
	snd, err := s.build(ctx, d)
	if err != nil {
		return err
	}
	if snd.close != nil {
		defer snd.close()
	}
	ctx, cancel := context.WithTimeout(ctx, s.d.Config.SendTimeout)
	defer cancel()
	now := s.d.Now().UTC()
	switch {
	case snd.logs != nil:
		return snd.logs(ctx, []LogRecord{NewLogRecord(telemetry.LogEntry{
			ResourceID: "service:export-test", Stream: "stdout", Timestamp: now, Message: "export destination test",
		})})
	case snd.audit != nil:
		b, err := newAuditBatch([]store.AuditEntry{{
			ID: "test_" + now.Format("20060102T150405Z"), ActorType: "system", ActorName: "export test", Ability: "root",
			Method: "TEST", Path: "/export/test", StatusCode: 200, CreatedAt: store.FormatAuditTime(now), ClientKind: "system", Action: "export.test",
		}})
		if err != nil {
			return err
		}
		return snd.audit.sendAudit(ctx, b)
	case snd.metrics != nil:
		series, _ := s.Series(ctx, nil)
		if len(series) == 0 {
			series = []Series{{Name: s.prefix + "export_test", Help: "Export destination test.", Value: 1, Time: now}}
		}
		body, err := EncodeOTLP(series, s.prefix[:len(s.prefix)-1], now)
		if err != nil {
			return err
		}
		return snd.metrics(ctx, body)
	}
	return errors.New("destination has no sender")
}

// Series returns the bounded metric set for scrape and OTLP push: per
// resource container metrics, platform gauges and exporter self metrics.
func (s *Service) Series(ctx context.Context, allow ResourceFilter) ([]Series, error) {
	var out []Series
	var firstErr error
	if s.d.Metrics != nil {
		rs, err := CollectResourceSeries(ctx, s.d.Metrics, s.prefix, allow)
		out, firstErr = append(out, rs...), err
	}
	if s.d.AppCount != nil {
		if n, err := s.d.AppCount(ctx); err == nil {
			out = append(out, Series{Name: s.prefix + "apps", Help: "Number of apps.", Value: float64(n)})
		}
	}
	return append(out, s.selfSeries(ctx)...), firstErr
}

func (s *Service) selfSeries(ctx context.Context) []Series {
	dests, err := s.d.Store.ListExportDestinations(ctx)
	if err != nil {
		return nil
	}
	var out []Series
	for _, d := range dests {
		st, err := s.Status(ctx, d.ID)
		if err != nil {
			continue
		}
		l := []Label{{"destination", d.Name}}
		add := func(name, help string, counter bool, v float64) {
			out = append(out, Series{Name: s.prefix + "export_" + name, Help: help, Counter: counter, Labels: l, Value: v})
		}
		add("sent_total", "Items delivered to the destination.", true, float64(st.SentTotal))
		add("dropped_total", "Items dropped because the queue overflowed or retries ran out.", true, float64(st.DroppedTotal))
		add("failures_total", "Failed delivery attempts.", true, float64(st.FailuresTotal))
		add("queue_depth", "Items waiting to be delivered.", false, float64(st.QueueDepth))
		open := 0.0
		if st.CircuitOpen {
			open = 1
		}
		add("circuit_open", "1 while the destination's circuit breaker is open.", false, open)
	}
	return out
}

// StreamAudit calls fn for every redacted audit row in [from, to), oldest
// first, up to the configured row cap.
func (s *Service) StreamAudit(ctx context.Context, from, to time.Time, fn func(AuditRecord) error) error {
	at := ""
	if !from.IsZero() {
		at = store.FormatAuditTime(from.Add(-time.Nanosecond))
	}
	id := ""
	var upper *time.Time
	if !to.IsZero() {
		upper = &to
	}
	total := 0
	for total < s.d.Config.AuditMaxRows {
		page := s.d.Config.AuditBatch
		if rest := s.d.Config.AuditMaxRows - total; rest < page {
			page = rest
		}
		rows, err := s.d.Store.ListAuditEntriesAfter(ctx, at, id, upper, page)
		if err != nil {
			return fmt.Errorf("dataexport: %w", err)
		}
		for _, e := range rows {
			if err := fn(NewAuditRecord(e)); err != nil {
				return err
			}
		}
		total += len(rows)
		if len(rows) < page {
			return nil
		}
		last := rows[len(rows)-1]
		at, id = last.CreatedAt, last.ID
	}
	return nil
}
