package deploylog

// State event kinds published on the all-attempts feed.
const (
	StateStarted  = "started"
	StateStep     = "step"
	StateFinished = "finished"
)

// StateEvent is one attempt lifecycle change on the all-attempts feed.
type StateEvent struct {
	AttemptID string
	Kind      string
	Step      string
	Status    string
}

// SubscribeAll returns a channel of every attempt's lifecycle events. Slow
// subscribers drop events rather than block the recorder.
func (r *Recorder) SubscribeAll() (<-chan StateEvent, func()) {
	ch := make(chan StateEvent, subscriberBufferSize)
	r.mu.Lock()
	r.feedSubs = append(r.feedSubs, ch)
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, c := range r.feedSubs {
			if c == ch {
				r.feedSubs = append(r.feedSubs[:i], r.feedSubs[i+1:]...)
				return
			}
		}
	}
}

func (r *Recorder) publishStateLocked(ev StateEvent) {
	for _, ch := range r.feedSubs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// StepSummary is the step-state tally of a live attempt.
type StepSummary struct {
	Done        int
	Running     int
	Failed      int
	FailingStep string
}

// StepSummaryFor tallies the latest state of each step of a live attempt;
// ok is false once the attempt finished (step history is not persisted).
func (r *Recorder) StepSummaryFor(attemptID string) (StepSummary, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.active[attemptID]
	if !ok {
		return StepSummary{}, false
	}
	latest := map[string]string{}
	var order []string
	for _, ev := range st.steps {
		if _, seen := latest[ev.Step]; !seen {
			order = append(order, ev.Step)
		}
		latest[ev.Step] = ev.Status
	}
	var sum StepSummary
	for _, name := range order {
		switch latest[name] {
		case "done":
			sum.Done++
		case "failed":
			sum.Failed++
			if sum.FailingStep == "" {
				sum.FailingStep = name
			}
		default:
			sum.Running++
		}
	}
	return sum, true
}
