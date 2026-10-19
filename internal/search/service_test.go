package search_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

type staticProvider struct {
	kind search.Kind
	docs []search.Doc
}

func (p staticProvider) Kind() search.Kind                          { return p.kind }
func (p staticProvider) Docs(context.Context) ([]search.Doc, error) { return p.docs, nil }

type allowFunc func(prefix, name, ability string) bool

func (f allowFunc) Allowed(prefix, name, ability string) bool { return f(prefix, name, ability) }

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func appDoc(name, body string, tags ...string) search.Doc {
	return search.Doc{Ref: "app:" + name, Kind: search.KindApp, Title: name, Body: body, Tags: tags, Env: "prod",
		Prefix: "app:", Name: name, Ability: "read", UpdatedAt: time.Now()}
}

func actionDocs(names ...string) []search.Doc {
	var out []search.Doc
	for _, n := range names {
		out = append(out, search.Doc{Ref: "action:app.restart:" + n, Kind: search.KindAction, Title: "Restart " + n,
			Body: "restart reboot", App: n, Prefix: "app:", Name: n, Ability: "deploy", Confirm: true, ActionID: "app.restart"})
	}
	return out
}

func refs(r search.Response) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, h.Ref)
	}
	return out
}

func TestParseQuery(t *testing.T) {
	q := search.ParseQuery("app:foo env:prod tag:x node:y kind:db restart api http://x:80")
	if q.App != "foo" || q.Env != "prod" || q.Tag != "x" || q.Node != "y" || q.Kind != search.KindDatabase {
		t.Fatalf("scopes = %+v", q)
	}
	if want := []string{"restart", "api", "http://x:80"}; !slices.Equal(q.Text, want) {
		t.Fatalf("text = %v, want %v", q.Text, want)
	}
	if !search.ParseQuery("  ").Empty() {
		t.Fatal("blank query should be empty")
	}
}

func TestSearchFiltersAndRanks(t *testing.T) {
	db := openDB(t)
	svc := search.New(db, nil,
		staticProvider{search.KindApp, []search.Doc{appDoc("api", "nginx:1", "backend"), appDoc("web", "node:20", "frontend"), appDoc("rapid-worker", "go")}},
		staticProvider{search.KindAction, actionDocs("api", "web")},
	)
	c := search.Caller{PrincipalID: "user:1", Authz: allowFunc(func(string, string, string) bool { return true })}
	ctx := context.Background()

	got, err := svc.Search(ctx, c, search.Request{Q: "restart api"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"action:app.restart:api"}; !slices.Equal(refs(got), want) {
		t.Fatalf("restart api = %v, want %v", refs(got), want)
	}
	if a := got.Hits[0].Action; a == nil || a.Ability != "deploy" || !a.Confirm {
		t.Fatalf("action info = %+v", got.Hits[0].Action)
	}

	got, _ = svc.Search(ctx, c, search.Request{Q: "api"})
	if len(got.Hits) < 2 || got.Hits[0].Ref != "app:api" {
		t.Fatalf("exact name should rank first: %v", refs(got))
	}

	got, _ = svc.Search(ctx, c, search.Request{Q: "tag:frontend"})
	if !slices.Equal(refs(got), []string{"app:web"}) {
		t.Fatalf("tag filter = %v", refs(got))
	}
	got, _ = svc.Search(ctx, c, search.Request{Q: "kind:action web"})
	if !slices.Equal(refs(got), []string{"action:app.restart:web"}) {
		t.Fatalf("kind filter = %v", refs(got))
	}
	got, _ = svc.Search(ctx, c, search.Request{Q: "ap"})
	if len(got.Hits) == 0 {
		t.Fatal("two letter query should fall back to substring match")
	}
	got, _ = svc.Search(ctx, c, search.Request{Q: "rapd"})
	if !slices.Contains(refs(got), "app:rapid-worker") {
		t.Fatalf("typo tolerance: %v", refs(got))
	}
}

func TestSearchPagingAndUsageBoost(t *testing.T) {
	db := openDB(t)
	var docs []search.Doc
	for i := range 30 {
		docs = append(docs, appDoc(fmt.Sprintf("svc-%02d", i), "common"))
	}
	svc := search.New(db, nil, staticProvider{search.KindApp, docs})
	c := search.Caller{PrincipalID: "user:1", Authz: allowFunc(func(string, string, string) bool { return true })}
	ctx := context.Background()

	p1, err := svc.Search(ctx, c, search.Request{Q: "common", Limit: 10})
	if err != nil || len(p1.Hits) != 10 || p1.NextCursor == "" || p1.Total != 30 {
		t.Fatalf("page 1: %+v %v", p1, err)
	}
	p2, _ := svc.Search(ctx, c, search.Request{Q: "common", Limit: 10, Cursor: p1.NextCursor})
	if slices.Contains(refs(p2), p1.Hits[0].Ref) {
		t.Fatal("page 2 repeated a page 1 hit")
	}
	last := "app:svc-29"
	for range 3 {
		if err := svc.RecordUse(ctx, c, last); err != nil {
			t.Fatal(err)
		}
	}
	boosted, _ := svc.Search(ctx, c, search.Request{Q: "common", Limit: 5})
	if boosted.Hits[0].Ref != last {
		t.Fatalf("used hit should lead, got %v", refs(boosted))
	}
	other, _ := svc.Search(ctx, search.Caller{PrincipalID: "user:2", Authz: c.Authz}, search.Request{Q: "common", Limit: 5})
	if other.Hits[0].Ref == last && other.Hits[1].Ref == "app:svc-00" {
		t.Fatal("usage must be per principal")
	}
}

func TestRecordUseHidesUnauthorizedRefs(t *testing.T) {
	db := openDB(t)
	svc := search.New(db, nil, staticProvider{search.KindApp, []search.Doc{appDoc("secret", "x")}})
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	deny := search.Caller{PrincipalID: "user:1", Authz: allowFunc(func(string, string, string) bool { return false })}
	if err := svc.RecordUse(context.Background(), deny, "app:secret"); err != search.ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestReconcileDropsRemovedDocs(t *testing.T) {
	db := openDB(t)
	p := &mutableProvider{docs: []search.Doc{appDoc("alpha", "x"), appDoc("beta", "x")}}
	svc := search.New(db, nil, p)
	c := search.Caller{PrincipalID: "u", Authz: allowFunc(func(string, string, string) bool { return true })}
	ctx := context.Background()
	if r, _ := svc.Search(ctx, c, search.Request{Q: "alpha"}); len(r.Hits) != 1 {
		t.Fatalf("want alpha: %v", refs(r))
	}
	p.docs = []search.Doc{appDoc("beta", "x")}
	if err := svc.Reindex(ctx, search.KindApp); err != nil {
		t.Fatal(err)
	}
	if r, _ := svc.Search(ctx, c, search.Request{Q: "alpha"}); len(r.Hits) != 0 {
		t.Fatalf("alpha should be gone: %v", refs(r))
	}
}

type mutableProvider struct{ docs []search.Doc }

func (p *mutableProvider) Kind() search.Kind                          { return search.KindApp }
func (p *mutableProvider) Docs(context.Context) ([]search.Doc, error) { return p.docs, nil }

func TestSavedSearchesAreOwnerScoped(t *testing.T) {
	db := openDB(t)
	svc := search.New(db, nil)
	ctx := context.Background()
	s, err := svc.SaveSearch(ctx, "user:1", "prod apis", "env:prod api")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.ListSaved(ctx, "user:2"); len(list) != 0 {
		t.Fatal("another principal must not see saved searches")
	}
	c := search.Caller{PrincipalID: "user:1", Authz: allowFunc(func(string, string, string) bool { return false })}
	r, _ := svc.Search(ctx, c, search.Request{Q: "prod"})
	if len(r.Hits) != 1 || r.Hits[0].Kind != search.KindSaved {
		t.Fatalf("owner should find saved search: %+v", r)
	}
	if err := svc.DeleteSaved(ctx, "user:2", s.ID); err != search.ErrNotFound {
		t.Fatalf("foreign delete = %v", err)
	}
	if err := svc.DeleteSaved(ctx, "user:1", s.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDocValidateRejectsMissingAuthzFields(t *testing.T) {
	good := appDoc("a", "")
	for name, mut := range map[string]func(*search.Doc){
		"no prefix":  func(d *search.Doc) { d.Prefix = "" },
		"bad prefix": func(d *search.Doc) { d.Prefix = "app" },
		"no name":    func(d *search.Doc) { d.Name = "" },
		"no ability": func(d *search.Doc) { d.Ability = "" },
	} {
		d := good
		mut(&d)
		if d.Validate() == nil {
			t.Errorf("%s: doc should be invalid", name)
		}
	}
	db := openDB(t)
	bad := good
	bad.Prefix = ""
	svc := search.New(db, nil, staticProvider{search.KindApp, []search.Doc{bad}})
	if err := svc.Reconcile(context.Background()); err == nil {
		t.Fatal("reconcile must reject a doc with no authorization fields")
	}
}

type recordingAuthz struct{ calls int }

func (r *recordingAuthz) Allowed(string, string, string) bool { r.calls++; return true }

func TestSearchCallsAuthorizerForEveryHit(t *testing.T) {
	db := openDB(t)
	svc := search.New(db, nil, staticProvider{search.KindApp, []search.Doc{appDoc("a1", "x"), appDoc("a2", "x"), appDoc("a3", "x")}})
	rec := &recordingAuthz{}
	r, err := svc.Search(context.Background(), search.Caller{PrincipalID: "u", Authz: rec}, search.Request{Q: "x"})
	if err != nil || len(r.Hits) != 3 || rec.calls < 3 {
		t.Fatalf("hits=%d authz calls=%d err=%v", len(r.Hits), rec.calls, err)
	}
}

func TestSearchCandidatesAreCapped(t *testing.T) {
	db := openDB(t)
	docs := make([]search.Doc, 0, 700)
	for i := range 700 {
		docs = append(docs, appDoc(fmt.Sprintf("svc-%03d", i), "shared body"))
	}
	svc := search.New(db, nil, staticProvider{search.KindApp, docs})
	c := search.Caller{PrincipalID: "user:1", Authz: allowFunc(func(string, string, string) bool { return true })}
	got, err := svc.Search(context.Background(), c, search.Request{Q: "shared", Limit: search.MaxPageSize})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total > 500 {
		t.Fatalf("one query ranked %d docs in memory, the candidate cap is 500", got.Total)
	}
}
