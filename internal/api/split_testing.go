package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/splittest"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SplitTestStore is the store surface split testing needs. The store passed to
// NewRouter may or may not implement it; routes answer 404 when it does not.
type SplitTestStore interface {
	CreateSplitTest(ctx context.Context, t store.SplitTest) error
	GetRunningSplitTest(ctx context.Context, app string) (store.SplitTest, bool, error)
	GetLatestSplitTest(ctx context.Context, app string) (store.SplitTest, bool, error)
	ListRunningSplitTests(ctx context.Context) ([]store.SplitTest, error)
	UpdateSplitTestVariants(ctx context.Context, id string, vs []splittest.Variant) error
	EndSplitTest(ctx context.Context, id, reason string) error
	AddSplitStats(ctx context.Context, testID string, counts map[string]uint64, at time.Time) error
	ListSplitStats(ctx context.Context, testID string) ([]store.SplitStat, error)
	ListDeployURLs(ctx context.Context, appName string) ([]store.DeployURL, error)
	ListBranchDeploysByApp(ctx context.Context, appName string) ([]store.BranchDeploy, error)
}

// splitError carries the HTTP status a split operation failed with.
type splitError struct {
	status int
	msg    string
}

func (e splitError) Error() string { return e.msg }

func splitErr(status int, format string, args ...any) error {
	return splitError{status: status, msg: fmt.Sprintf(format, args...)}
}

// splitService holds the logic of split testing behind small function fields,
// so tests can fail any step (deploy, end, lookup) without a real router.
type splitService struct {
	st       SplitTestStore
	reg      *ingress.Splits
	defaults splittest.Defaults
	logger   *slog.Logger
	now      func() time.Time

	appDomains   func(ctx context.Context, app string) ([]string, error)
	serviceImage func(ctx context.Context, service string) (string, error)
	lockReason   func(ctx context.Context, app string) (string, bool)
	deployImage  func(ctx context.Context, app, image string) error
	publicURL    func(ctx context.Context, host string) string
	nudge        func()
}

type splitVariantInput struct {
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	Ref    string `json:"ref"`
	Weight int    `json:"weight"`
}

type splitStartRequest struct {
	Variants   []splitVariantInput `json:"variants"`
	CookieName string              `json:"cookie_name,omitempty"`
	CookieTTL  string              `json:"cookie_ttl,omitempty"`
}

type splitWeightsRequest struct {
	Weights map[string]int `json:"weights"`
}

type splitPromoteRequest struct {
	Variant string `json:"variant"`
}

// splitHostFor resolves the public host serving a non-production variant.
func (s *splitService) splitHostFor(ctx context.Context, app string, v splittest.Variant) (host, service string) {
	switch v.Kind {
	case splittest.KindBranch:
		rows, err := s.st.ListBranchDeploysByApp(ctx, app)
		if err != nil {
			return "", ""
		}
		for _, b := range rows {
			if (b.Branch == v.Ref || b.Slug == v.Ref) && b.Domain != "" {
				return b.Domain, b.DeployAppID
			}
		}
	case splittest.KindRelease:
		rows, err := s.st.ListDeployURLs(ctx, app)
		if err != nil {
			return "", ""
		}
		for _, r := range rows {
			if r.ServiceName == v.Ref && r.Host != "" {
				return r.Host, r.ServiceName
			}
		}
	}
	return "", ""
}

func newSplitVariantID(testID string, n int) string {
	prefix := testID
	if len(prefix) > 4 {
		prefix = prefix[:4]
	}
	if n == 0 {
		return prefix + "-p"
	}
	return fmt.Sprintf("%s-%d", prefix, n)
}

// Start creates and activates a test. Production is added with the leftover
// weight when the request lists only the other arms.
func (s *splitService) Start(ctx context.Context, app string, req splitStartRequest) (splitTestResource, error) {
	domains, err := s.appDomains(ctx, app)
	if err != nil {
		return splitTestResource{}, err
	}
	if len(domains) == 0 {
		return splitTestResource{}, splitErr(http.StatusConflict, "the app has no domain to split traffic on, add a domain first")
	}
	testID, err := store.NewSplitTestID()
	if err != nil {
		return splitTestResource{}, err
	}
	vs := buildSplitVariants(testID, req.Variants)
	if err := splittest.Validate(vs); err != nil {
		return splitTestResource{}, splitErr(http.StatusBadRequest, "%s", err.Error())
	}
	for _, v := range vs {
		if v.Kind == splittest.KindProduction {
			continue
		}
		if host, _ := s.splitHostFor(ctx, app, v); host == "" {
			return splitTestResource{}, splitErr(http.StatusBadRequest, "variant %q: no ready %s deploy %q for this app", v.Label, v.Kind, v.Ref)
		}
	}
	cookie, ttl, err := s.cookieSettings(req)
	if err != nil {
		return splitTestResource{}, splitErr(http.StatusBadRequest, "%s", err.Error())
	}
	t := store.SplitTest{ID: testID, AppName: app, Status: splittest.StatusRunning, Variants: vs, CookieName: cookie, CookieTTLSecs: int(ttl.Seconds())}
	if err := s.st.CreateSplitTest(ctx, t); err != nil {
		if errors.Is(err, store.ErrSplitTestRunning) {
			return splitTestResource{}, splitErr(http.StatusConflict, "a split test is already running for this app, stop or promote it first")
		}
		return splitTestResource{}, err
	}
	s.activate(ctx, t, domains)
	s.nudge()
	return s.Get(ctx, app)
}

func (s *splitService) cookieSettings(req splitStartRequest) (string, time.Duration, error) {
	name, ttl := s.defaults.CookieName, s.defaults.CookieTTL
	if req.CookieName != "" {
		if !splittest.ValidCookieName(req.CookieName) {
			return "", 0, errors.New("cookie_name may only contain letters, digits, dash and underscore")
		}
		name = req.CookieName
	}
	if req.CookieTTL != "" {
		d, err := time.ParseDuration(req.CookieTTL)
		if err != nil || d < time.Minute {
			return "", 0, errors.New("cookie_ttl must be a duration of at least 1m, such as 720h")
		}
		ttl = d
	}
	return name, ttl, nil
}

func buildSplitVariants(testID string, in []splitVariantInput) []splittest.Variant {
	var out []splittest.Variant
	sum := 0
	hasProd := false
	for _, v := range in {
		kind := strings.TrimSpace(v.Kind)
		if kind == splittest.KindProduction {
			hasProd = true
		}
		label := strings.TrimSpace(v.Label)
		if label == "" {
			label = strings.TrimSpace(v.Ref)
		}
		sum += v.Weight
		out = append(out, splittest.Variant{Label: label, Kind: kind, Ref: strings.TrimSpace(v.Ref), Weight: v.Weight})
	}
	if !hasProd {
		out = append([]splittest.Variant{{Label: "Production", Kind: splittest.KindProduction, Weight: max(0, splittest.TotalWeight-sum)}}, out...)
	}
	next := 1
	for i := range out {
		if out[i].Kind == splittest.KindProduction {
			out[i].ID = newSplitVariantID(testID, 0)
			continue
		}
		out[i].ID = newSplitVariantID(testID, next)
		next++
	}
	return out
}

// activeVariants hands ingress the test's variants with unreachable arms at
// weight 0 and their share moved to production, so a deleted branch deploy
// can never send visitors to an error page.
func (s *splitService) activeVariants(ctx context.Context, t store.SplitTest) ([]splittest.Variant, map[string]string) {
	vs := slices.Clone(t.Variants)
	hosts := map[string]string{}
	moved := 0
	for i, v := range vs {
		if v.Kind == splittest.KindProduction {
			continue
		}
		host, _ := s.splitHostFor(ctx, t.AppName, v)
		if host == "" {
			moved += v.Weight
			vs[i].Weight = 0
			continue
		}
		hosts[v.ID] = host
	}
	for i, v := range vs {
		if v.Kind == splittest.KindProduction {
			vs[i].Weight += moved
		}
	}
	return vs, hosts
}

func (s *splitService) activate(ctx context.Context, t store.SplitTest, domains []string) {
	vs, hosts := s.activeVariants(ctx, t)
	s.reg.Set(domains, ingress.ActiveSplit{
		TestID: t.ID, CookieName: t.CookieName, CookieTTL: time.Duration(t.CookieTTLSecs) * time.Second,
		OverrideParam: s.defaults.OverrideParam, Variants: vs, VariantHosts: hosts,
	})
}

// Sync rebuilds the ingress registry from the running tests, at boot and on
// every flush tick, so a variant whose deploy vanished stops receiving traffic.
func (s *splitService) Sync(ctx context.Context) (changed bool) {
	tests, err := s.st.ListRunningSplitTests(ctx)
	if err != nil {
		s.logger.Warn("api: split test sync failed", slog.String("error", err.Error()))
		return false
	}
	for _, t := range tests {
		domains, err := s.appDomains(ctx, t.AppName)
		if err != nil || len(domains) == 0 {
			continue
		}
		if !s.reg.Active(domains) {
			changed = true
		}
		s.activate(ctx, t, domains)
	}
	return changed
}

// Flush moves the in-memory request counts into the database.
func (s *splitService) Flush(ctx context.Context) {
	for testID, counts := range s.reg.Drain() {
		if err := s.st.AddSplitStats(ctx, testID, counts, s.now()); err != nil {
			s.logger.Warn("api: persist split stats failed", slog.String("test", testID), slog.String("error", err.Error()))
		}
	}
}

// SetWeights changes the weights of the running test. Visitors keep their
// variant unless it drops to weight 0.
func (s *splitService) SetWeights(ctx context.Context, app string, req splitWeightsRequest) (splitTestResource, error) {
	t, ok, err := s.st.GetRunningSplitTest(ctx, app)
	if err != nil {
		return splitTestResource{}, err
	}
	if !ok {
		return splitTestResource{}, splitErr(http.StatusNotFound, "no split test is running for this app")
	}
	vs := slices.Clone(t.Variants)
	for key, w := range req.Weights {
		v, found := splittest.Find(vs, key)
		if !found {
			return splitTestResource{}, splitErr(http.StatusBadRequest, "unknown variant %q", key)
		}
		for i := range vs {
			if vs[i].ID == v.ID {
				vs[i].Weight = w
			}
		}
	}
	if err := splittest.Validate(vs); err != nil {
		return splitTestResource{}, splitErr(http.StatusBadRequest, "%s", err.Error())
	}
	if err := s.st.UpdateSplitTestVariants(ctx, t.ID, vs); err != nil {
		return splitTestResource{}, err
	}
	t.Variants = vs
	if domains, err := s.appDomains(ctx, app); err == nil {
		s.activate(ctx, t, domains)
	}
	return s.Get(ctx, app)
}

// Stop ends the test; every visitor then gets production. The database is
// updated first, so a failure leaves the split running and retryable.
func (s *splitService) Stop(ctx context.Context, app, reason string) (splitTestResource, error) {
	t, ok, err := s.st.GetRunningSplitTest(ctx, app)
	if err != nil {
		return splitTestResource{}, err
	}
	if !ok {
		return splitTestResource{}, splitErr(http.StatusNotFound, "no split test is running for this app")
	}
	s.Flush(ctx)
	if err := s.st.EndSplitTest(ctx, t.ID, reason); err != nil {
		return splitTestResource{}, err
	}
	s.reg.Clear(t.ID)
	s.nudge()
	return s.Get(ctx, app)
}

// Promote makes a variant the production deploy through the normal deploy
// path, then ends the test. It refuses while publishing is locked. A failed
// deploy leaves the test running; a failed end after a good deploy is
// retryable because deploying an image the app already runs is a no-op.
func (s *splitService) Promote(ctx context.Context, app, variantKey string) (splitTestResource, error) {
	t, ok, err := s.st.GetRunningSplitTest(ctx, app)
	if err != nil {
		return splitTestResource{}, err
	}
	if !ok {
		return splitTestResource{}, splitErr(http.StatusNotFound, "no split test is running for this app")
	}
	v, found := splittest.Find(t.Variants, variantKey)
	if !found {
		return splitTestResource{}, splitErr(http.StatusBadRequest, "unknown variant %q", variantKey)
	}
	if v.Kind == splittest.KindProduction {
		return splitTestResource{}, splitErr(http.StatusBadRequest, "production is already live, stop the test instead")
	}
	if reason, locked := s.lockReason(ctx, app); locked {
		return splitTestResource{}, splitErr(http.StatusConflict, "publishing is locked for this app, unlock it before promoting: %s", reason)
	}
	_, service := s.splitHostFor(ctx, app, v)
	if service == "" {
		return splitTestResource{}, splitErr(http.StatusConflict, "variant %q has no running deploy to promote", v.Label)
	}
	image, err := s.serviceImage(ctx, service)
	if err != nil || image == "" {
		return splitTestResource{}, splitErr(http.StatusConflict, "variant %q has no image to promote", v.Label)
	}
	if err := s.deployImage(ctx, app, image); err != nil {
		var se splitError
		if errors.As(err, &se) {
			return splitTestResource{}, se
		}
		return splitTestResource{}, splitErr(http.StatusBadGateway, "deploying the winner failed, the test is still running: %s", err.Error())
	}
	s.Flush(ctx)
	if err := s.st.EndSplitTest(ctx, t.ID, "promoted: "+v.Label); err != nil {
		return splitTestResource{}, splitErr(http.StatusBadGateway, "the winner was deployed but the test could not be ended, retry promote to finish: %s", err.Error())
	}
	s.reg.Clear(t.ID)
	s.nudge()
	return s.Get(ctx, app)
}
