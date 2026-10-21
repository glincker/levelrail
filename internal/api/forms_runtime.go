package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/forms"
	"github.com/GLINCKER/levelrail/internal/store"
)

// FormsStore is the persistence surface for form capture; nil on the router means the routes answer 501.
type FormsStore interface {
	forms.Store
	CreateForm(ctx context.Context, f store.Form) error
	UpdateForm(ctx context.Context, f store.Form) error
	GetForm(ctx context.Context, app, name string) (store.Form, error)
	ListForms(ctx context.Context, app string) ([]store.Form, map[string]store.FormCounts, error)
	DeleteForm(ctx context.Context, id string) error
	ListFormSubmissions(ctx context.Context, formID string, q store.FormSubmissionQuery) ([]store.FormSubmission, error)
	SetFormSubmissionSpam(ctx context.Context, formID string, id int64, spam bool, reason string) error
	DeleteFormSubmission(ctx context.Context, formID string, id int64) error
}

func formsStoreOf(s Store) FormsStore {
	fs, _ := s.(FormsStore)
	return fs
}

// formsRuntime is built on first use so options such as the notifier are already applied.
type formsRuntime struct {
	once    sync.Once
	svc     *forms.Service
	cfg     forms.Config
	perIP   *apiRateLimiter
	perForm *apiRateLimiter
}

func (rt *Router) formsRuntime() *formsRuntime {
	fr := rt.formsRT
	fr.once.Do(func() {
		fr.cfg = forms.LoadConfig(os.LookupEnv)
		fr.perIP = newAPIRateLimiter(fr.cfg.RatePerIPPerMinute)
		fr.perForm = newAPIRateLimiter(fr.cfg.RatePerFormPerMin)
		var n forms.Notifier
		if rt.deviceNotifier != nil && rt.notificationChannels != nil {
			n = formNotifier{rt: rt}
		}
		fr.svc = forms.NewService(rt.forms, n, fr.cfg, rt.logger)
	})
	return fr
}

// formNotifier sends a new-submission notice through each channel the form lists.
type formNotifier struct{ rt *Router }

func (n formNotifier) Notify(ctx context.Context, f store.Form, s store.FormSubmission) error {
	text := forms.NoticeText(f, s, formsDashboardLink(n.rt, f))
	var errs []error
	for _, id := range f.NotifyChannelIDs {
		c, err := n.rt.notificationChannels.GetNotificationChannel(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", id, err))
			continue
		}
		if !c.Enabled {
			continue
		}
		if err := n.rt.deviceNotifier.SendNotice(ctx, *c, text); err != nil {
			errs = append(errs, fmt.Errorf("channel %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func formsDashboardLink(rt *Router, f store.Form) string {
	base := rt.dashboardURL()
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/apps/" + f.AppName + "/forms"
}
