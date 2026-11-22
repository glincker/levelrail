package preview

import (
	"context"
	"time"
)

// Live is the part of the configuration an operator can change at runtime.
// The Manager reads it before each decision, so a change applies at once.
type Live struct {
	DefaultMode   Mode
	MaxConcurrent int
	MinInterval   time.Duration
	RenderTimeout time.Duration
	IdleStop      time.Duration
	MemoryMB      int64
	CPUs          float64
}

// LiveSource returns the current Live values.
type LiveSource func(ctx context.Context) Live

// WithLive makes the Manager read its runtime settings from src.
func WithLive(src LiveSource) Option { return func(m *Manager) { m.liveSrc = src } }

// live returns the runtime settings, seeded from Config when no source is wired.
func (m *Manager) live(ctx context.Context) Live {
	l := Live{
		DefaultMode: m.cfg.DefaultMode, MaxConcurrent: m.cfg.MaxConcurrent, MinInterval: m.cfg.MinInterval,
		RenderTimeout: m.cfg.Timeout, IdleStop: m.cfg.IdleStop, MemoryMB: m.cfg.MemoryMB, CPUs: m.cfg.CPUs,
	}
	if m.liveSrc != nil {
		l = m.liveSrc(ctx)
	}
	if l.MaxConcurrent < 1 {
		l.MaxConcurrent = 1
	}
	if l.RenderTimeout <= 0 {
		l.RenderTimeout = m.cfg.Timeout
	}
	return l
}
