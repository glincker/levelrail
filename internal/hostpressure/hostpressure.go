package hostpressure

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// State is how stressed the host is, worst last.
type State string

// States.
const (
	StateOK       State = "ok"
	StateWarn     State = "warn"
	StateCritical State = "critical"
)

func (s State) rank() int {
	switch s {
	case StateCritical:
		return 2
	case StateWarn:
		return 1
	}
	return 0
}

// Rank is 0 for ok, 1 for warn and 2 for critical, for gauges.
func (s State) Rank() int { return s.rank() }

// WaitMessage is the status shown while a new build waits for the host to recover.
const WaitMessage = "waiting: host under memory pressure"

// consumerTimeout bounds the top consumer lookup so a slow engine cannot stall the sampler.
const consumerTimeout = 5 * time.Second

// Config holds the live thresholds. Percent fields are 0 to 100, load is load1 per CPU times 100.
type Config struct {
	Enabled     bool
	ShedEnabled bool

	CPUWarn, CPUCritical         float64 // cpu some avg10
	MemWarn, MemCritical         float64 // memory some avg10
	MemFullWarn, MemFullCritical float64 // memory full avg10
	AvailWarn, AvailCritical     float64 // MemAvailable as percent of total, lower is worse
	LoadWarn, LoadCritical       float64 // used only without PSI

	// ExitPercent scales every enter threshold to its exit threshold so a state leaves only after it clearly eased.
	ExitPercent float64
	Hold        time.Duration
	Jitter      time.Duration
	Interval    time.Duration
	NoSwapRAM   int64 // bytes: flag missing swap below this much RAM
}

// Consumer is one container's memory use.
type Consumer struct {
	Name     string `json:"name"`
	MemBytes int64  `json:"mem_bytes"`
}

// ConsumerFunc lists the biggest memory users; it must be bounded.
type ConsumerFunc func(ctx context.Context) ([]Consumer, error)

// Status is the last evaluation.
type Status struct {
	State        State      `json:"state"`
	Since        time.Time  `json:"since"`
	Source       string     `json:"source"` // psi, fallback or unavailable
	Reasons      []string   `json:"reasons"`
	Sample       Sample     `json:"sample"`
	TopConsumers []Consumer `json:"top_consumers"`
	Actions      []string   `json:"actions"`
	NoSwap       bool       `json:"no_swap"`
}

// Guard evaluates samples with hysteresis and gates new builds.
type Guard struct {
	read      ReadFunc
	cfg       func() Config
	log       *slog.Logger
	now       func() time.Time
	consumers ConsumerFunc
	rand      func(limit time.Duration) time.Duration

	// OnChange runs after the state changes, outside the lock.
	OnChange func(Status)
	// Sink receives every evaluation, for gauges.
	Sink func(Status)

	mu         sync.Mutex
	st         Status
	belowSince time.Time
	changed    chan struct{}
}

// New builds a Guard that starts in ok.
func New(read ReadFunc, cfg func() Config, logger *slog.Logger) *Guard {
	if logger == nil {
		logger = slog.Default()
	}
	return &Guard{
		read: read, cfg: cfg, log: logger, now: time.Now, changed: make(chan struct{}),
		st: Status{State: StateOK, Source: "unavailable"},
		rand: func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return rand.N(limit) //nolint:gosec,forbidigo // jitter, not security
		},
	}
}

// SetConsumers wires the top consumer lookup run on each escalation.
func (g *Guard) SetConsumers(f ConsumerFunc) { g.consumers = f }

// Status returns the last evaluation.
func (g *Guard) Status() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.st
}

type signal struct {
	name       string
	val        float64
	warn, crit float64
	low        bool // lower value is worse
}

func signals(s Sample, c Config) []signal {
	var out []signal
	if s.MemTotal > 0 {
		out = append(out, signal{"memory available", float64(s.MemAvailable) * 100 / float64(s.MemTotal), c.AvailWarn, c.AvailCritical, true})
	}
	if s.PSI {
		out = append(out,
			signal{"cpu pressure", s.CPUSome10, c.CPUWarn, c.CPUCritical, false},
			signal{"memory pressure", s.MemSome10, c.MemWarn, c.MemCritical, false},
			signal{"memory stall", s.MemFull10, c.MemFullWarn, c.MemFullCritical, false})
	} else if s.NumCPU > 0 {
		out = append(out, signal{"load per cpu", s.Load1 * 100 / float64(s.NumCPU), c.LoadWarn, c.LoadCritical, false})
	}
	return out
}

// level is the worst state any signal reaches when its thresholds are scaled to pct percent.
func level(sigs []signal, pct float64) (State, []string) {
	worst := StateOK
	var why []string
	for _, sg := range sigs {
		st := StateOK
		for _, t := range []struct {
			v  float64
			st State
		}{{sg.crit, StateCritical}, {sg.warn, StateWarn}} {
			if t.v <= 0 {
				continue
			}
			th := t.v * pct / 100
			if sg.low {
				th = t.v * (200 - pct) / 100
			}
			if (!sg.low && sg.val >= th) || (sg.low && sg.val <= th) {
				st = t.st
				break
			}
		}
		if st.rank() > worst.rank() {
			worst, why = st, nil
		}
		if st == worst && st != StateOK {
			why = append(why, fmt.Sprintf("%s %.0f%%", sg.name, sg.val))
		}
	}
	return worst, why
}

// Observe takes one reading and applies it. Escalation is immediate; stepping down needs Hold below the exit thresholds.
func (g *Guard) Observe(ctx context.Context) Status {
	c := g.cfg()
	if !c.Enabled {
		return g.Status()
	}
	s, err := g.read()
	if err != nil {
		g.mu.Lock()
		g.st.Source = "unavailable"
		out := g.st
		g.mu.Unlock()
		return out
	}
	sigs := signals(s, c)
	enter, why := level(sigs, 100)
	holdLvl, _ := level(sigs, c.ExitPercent)
	now := g.now()

	g.mu.Lock()
	prev := g.st.State
	cur := prev
	switch {
	case enter.rank() > cur.rank():
		cur, g.belowSince = enter, time.Time{}
	case holdLvl.rank() >= cur.rank():
		g.belowSince = time.Time{}
	default:
		if g.belowSince.IsZero() {
			g.belowSince = now
		}
		if now.Sub(g.belowSince) >= c.Hold {
			cur, g.belowSince = holdLvl, time.Time{}
		}
	}
	g.st.Source = "fallback"
	if s.PSI {
		g.st.Source = "psi"
	}
	g.st.Sample = s
	g.st.NoSwap = s.SwapTotal == 0 && s.MemTotal > 0 && s.MemTotal < c.NoSwapRAM
	changed := cur != prev
	if changed {
		g.st.State, g.st.Since = cur, now
		close(g.changed)
		g.changed = make(chan struct{})
		g.st.Actions = actionsFor(cur, g.st.NoSwap)
	}
	if cur != StateOK {
		g.st.Reasons = why
	} else {
		g.st.Reasons, g.st.TopConsumers = nil, nil
	}
	escalated := changed && cur.rank() > prev.rank()
	g.mu.Unlock()

	if escalated && g.consumers != nil {
		cctx, cancel := context.WithTimeout(ctx, consumerTimeout)
		top, cerr := g.consumers(cctx)
		cancel()
		if cerr != nil {
			g.log.Warn("hostpressure: top consumers unavailable", slog.String("error", cerr.Error()))
		}
		g.mu.Lock()
		g.st.TopConsumers = top
		g.mu.Unlock()
	}
	out := g.Status()
	if g.Sink != nil {
		g.Sink(out)
	}
	if changed && g.OnChange != nil {
		g.OnChange(out)
	}
	return out
}

func actionsFor(s State, noSwap bool) []string {
	if s == StateOK {
		return nil
	}
	a := []string{"Lower build concurrency in the build limits so fewer builds run at once", "Set memory limits on the apps listed as top consumers"}
	if noSwap {
		a = append([]string{"Add swap (a 1 to 2 GB swap file) so a memory spike slows the host instead of killing processes"}, a...)
	}
	return a
}

// Run observes on the configured interval until ctx ends.
func (g *Guard) Run(ctx context.Context) {
	for {
		g.Observe(ctx)
		d := g.cfg().Interval
		if d <= 0 {
			d = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

// BuildCap lowers a build concurrency limit to 1 while the host is warm or worse; 0 (unlimited) is capped too.
func (g *Guard) BuildCap(limit int) int {
	c := g.cfg()
	if !c.Enabled || !c.ShedEnabled || g.Status().State == StateOK {
		return limit
	}
	return 1
}

// Admit blocks a new build while the host is critical, calls wait once with WaitMessage, then resumes after a random jitter.
// It fails only when ctx ends and never touches work that is already running.
func (g *Guard) Admit(ctx context.Context, wait func(msg string)) error {
	announced := false
	for {
		c := g.cfg()
		g.mu.Lock()
		hold := c.Enabled && c.ShedEnabled && g.st.State == StateCritical
		ch := g.changed
		g.mu.Unlock()
		if !hold {
			return nil
		}
		if !announced && wait != nil {
			wait(WaitMessage)
			announced = true
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("hostpressure: wait canceled: %w", ctx.Err())
		case <-ch:
		}
		if j := g.rand(c.Jitter); j > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("hostpressure: wait canceled: %w", ctx.Err())
			case <-time.After(j):
			}
		}
	}
}
