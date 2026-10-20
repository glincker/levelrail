package docker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// PullPhase says what a PullEvent reports.
type PullPhase string

// Pull event phases.
const (
	PullPhaseStart    PullPhase = "start"
	PullPhaseProgress PullPhase = "progress"
	PullPhaseLayer    PullPhase = "layer"
	PullPhaseDone     PullPhase = "done"
)

// PullStatusCached is PullEvent.Status when the image was already local.
const PullStatusCached = "cached"

// Layer states derived from the daemon's pull stream.
const (
	LayerQueued      = "queued"
	LayerDownloading = "downloading"
	LayerDownloaded  = "downloaded"
	LayerExtracting  = "extracting"
	LayerComplete    = "complete"
	LayerExists      = "already exists"
)

// PullEvent is one throttled report from an image pull.
type PullEvent struct {
	Phase PullPhase
	Ref   string
	// Aggregate view, set on Progress and Done.
	Layers, LayersDone, Extracting, Existing int
	BytesCurrent, BytesTotal                 int64
	Percent                                  int
	// Layer view, set on PullPhaseLayer.
	LayerID, LayerState string
	LayerCurrent        int64
	LayerTotal          int64
	// Final status line and digest, set on Done.
	Status string
	Digest string
}

// PullConfig throttles pull reporting so a many-layer image cannot flood a log.
type PullConfig struct {
	Interval     time.Duration
	MinPercent   int
	MaxLayerLine int
}

// Environment variables behind PullConfig.
const (
	EnvPullInterval     = "APP_DEPLOY_TRACE_PULL_INTERVAL"
	EnvPullMinPercent   = "APP_DEPLOY_TRACE_PULL_MIN_PERCENT"
	EnvPullMaxLayerLine = "APP_DEPLOY_TRACE_PULL_MAX_LAYER_LINES"
)

// PullConfigFromEnv reads the throttle settings; bad values keep the default.
func PullConfigFromEnv(lookup func(string) (string, bool)) PullConfig {
	cfg := PullConfig{Interval: 2 * time.Second, MinPercent: 10, MaxLayerLine: 12}
	if v, ok := lookup(EnvPullInterval); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d >= 0 {
			cfg.Interval = d
		}
	}
	if v, ok := lookup(EnvPullMinPercent); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 && n <= 100 {
			cfg.MinPercent = n
		}
	}
	if v, ok := lookup(EnvPullMaxLayerLine); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			cfg.MaxLayerLine = n
		}
	}
	return cfg
}

type pullMessage struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
	ErrorDetail struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
}

type pullLayer struct {
	state          string
	current, total int64
}

// PullTracker folds the daemon's JSON pull stream into throttled events.
type PullTracker struct {
	cfg    PullConfig
	ref    string
	emit   func(PullEvent)
	now    func() time.Time
	layers map[string]*pullLayer
	order  []string

	lastEmit    time.Time
	lastPercent int
	layerLines  int
	malformed   int
	digest      string
	status      string
}

// NewPullTracker builds a tracker that reports through emit.
func NewPullTracker(ref string, cfg PullConfig, emit func(PullEvent)) *PullTracker {
	return &PullTracker{cfg: cfg, ref: ref, emit: emit, now: time.Now, layers: map[string]*pullLayer{}, lastPercent: -1}
}

// Malformed counts stream lines that were not valid JSON.
func (p *PullTracker) Malformed() int { return p.malformed }

// Consume reads the whole stream. A daemon-reported error (auth denied, a
// layer failing mid-download) is returned with the daemon's own text.
func (p *PullTracker) Consume(r io.Reader) error {
	p.emit(PullEvent{Phase: PullPhaseStart, Ref: p.ref})
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, readErr := br.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if err := p.handle(line); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fmt.Errorf("reading progress stream: %w", readErr)
		}
	}
	ev := p.aggregate(PullPhaseDone)
	ev.Status, ev.Digest = p.status, p.digest
	p.emit(ev)
	return nil
}

func (p *PullTracker) handle(line []byte) error {
	var m pullMessage
	if err := json.Unmarshal(line, &m); err != nil {
		p.malformed++
		return nil
	}
	if m.Error != "" || m.ErrorDetail.Message != "" {
		msg := m.Error
		if msg == "" {
			msg = m.ErrorDetail.Message
		}
		return errors.New(msg)
	}
	if m.ID == "" {
		switch {
		case strings.HasPrefix(m.Status, "Digest: "):
			p.digest = strings.TrimPrefix(m.Status, "Digest: ")
		case strings.HasPrefix(m.Status, "Status: "):
			p.status = strings.TrimPrefix(m.Status, "Status: ")
		}
		return nil
	}
	l, seen := p.layers[m.ID]
	if !seen {
		l = &pullLayer{state: LayerQueued}
		p.layers[m.ID] = l
		p.order = append(p.order, m.ID)
	}
	prev := l.state
	switch strings.ToLower(m.Status) {
	case "pulling fs layer", "waiting":
		l.state = LayerQueued
	case "downloading":
		l.state = LayerDownloading
		l.current, l.total = m.ProgressDetail.Current, m.ProgressDetail.Total
	case "verifying checksum", "download complete":
		l.state = LayerDownloaded
		l.current = l.total
	case "extracting":
		l.state = LayerExtracting
		if m.ProgressDetail.Total > 0 {
			l.total = max(l.total, m.ProgressDetail.Total)
		}
	case "pull complete":
		l.state = LayerComplete
		l.current = l.total
	case "already exists":
		l.state = LayerExists
	}
	if l.state != prev && (l.state == LayerExtracting || l.state == LayerComplete || l.state == LayerExists) {
		p.layerEvent(m.ID, l)
	}
	p.maybeProgress()
	return nil
}

func (p *PullTracker) layerEvent(id string, l *pullLayer) {
	if p.layerLines >= p.cfg.MaxLayerLine {
		return
	}
	p.layerLines++
	p.emit(PullEvent{Phase: PullPhaseLayer, Ref: p.ref, LayerID: id, LayerState: l.state, LayerCurrent: l.current, LayerTotal: l.total})
}

func (p *PullTracker) maybeProgress() {
	ev := p.aggregate(PullPhaseProgress)
	stepped := p.lastPercent < 0 || ev.Percent-p.lastPercent >= p.cfg.MinPercent
	timed := p.now().Sub(p.lastEmit) >= p.cfg.Interval
	if !stepped && !timed {
		return
	}
	if ev.Percent == p.lastPercent && !timed {
		return
	}
	p.lastPercent, p.lastEmit = ev.Percent, p.now()
	p.emit(ev)
}

func (p *PullTracker) aggregate(phase PullPhase) PullEvent {
	ev := PullEvent{Phase: phase, Ref: p.ref, Layers: len(p.layers)}
	for _, id := range p.order {
		l := p.layers[id]
		switch l.state {
		case LayerComplete:
			ev.LayersDone++
		case LayerExists:
			ev.LayersDone++
			ev.Existing++
		case LayerExtracting:
			ev.Extracting++
		}
		if l.state == LayerExists {
			continue
		}
		ev.BytesTotal += l.total
		ev.BytesCurrent += min(l.current, l.total)
	}
	switch {
	case phase == PullPhaseDone:
		ev.Percent = 100
	case ev.BytesTotal > 0:
		ev.Percent = int(min(99, ev.BytesCurrent*100/ev.BytesTotal))
	}
	return ev
}
