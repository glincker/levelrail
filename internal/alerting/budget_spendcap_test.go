package alerting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

type capBudgetSource struct {
	breaches []spendcap.Breach
	err      error
}

func (capBudgetSource) BudgetStatuses(context.Context) ([]usagecost.BudgetStatus, error) {
	return nil, nil
}

func (s capBudgetSource) SpendCapBreaches(context.Context) ([]spendcap.Breach, error) {
	return s.breaches, s.err
}

func TestEvaluateBudgetThresholdIncludesSpendCaps(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	r := Rule{ID: "budget", Kind: KindBudgetThreshold}
	src := capBudgetSource{breaches: []spendcap.Breach{{Key: "spendcap:web:2026-10:80", Text: `app "web" has used 80% of its monthly limit`}}}

	next, notices, keys, err := EvaluateBudgetThreshold(context.Background(), src, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || len(keys) != 1 || keys[0] != "spendcap:web:2026-10:80" || !next.Firing && next.PendingSince == nil {
		t.Errorf("notices=%v keys=%v next=%+v", notices, keys, next)
	}

	_, notices, _, err = EvaluateBudgetThreshold(context.Background(), capBudgetSource{}, r, now)
	if err != nil || len(notices) != 0 {
		t.Errorf("no breaches should not notify: %v %v", notices, err)
	}

	if _, _, _, err = EvaluateBudgetThreshold(context.Background(), capBudgetSource{err: errors.New("boom")}, r, now); err == nil {
		t.Error("spend cap read failure swallowed")
	}
}
