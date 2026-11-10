package alerting

import (
	"testing"
	"time"
)

func TestLogRulePolicySourceOverridesBoot(t *testing.T) {
	l := newLogRuleRuntime()
	if l.current().MaxRows != DefaultLogRuleMaxRows {
		t.Fatalf("boot policy = %+v", l.current())
	}
	l.source = func() LogRulePolicy { return LogRulePolicy{EvalEvery: time.Second, MaxRows: 9} }
	if got := l.current(); got.MaxRows != 9 || got.EvalEvery != time.Second {
		t.Fatalf("live policy = %+v", got)
	}
}
