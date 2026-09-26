package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestApplyDomainEdit(t *testing.T) {
	tests := []struct {
		name         string
		current      []string
		add, remove  []string
		want         []string
		wantNotFound bool
	}{
		{"add new", []string{"a.com"}, []string{"b.com"}, nil, []string{"a.com", "b.com"}, false},
		{"add existing is a no-op", []string{"a.com"}, []string{"a.com"}, nil, []string{"a.com"}, false},
		{"remove", []string{"a.com", "b.com"}, nil, []string{"a.com"}, []string{"b.com"}, false},
		{"remove missing", []string{"a.com"}, nil, []string{"z.com"}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyDomainEdit(tt.current, tt.add, tt.remove)
			if (err != nil) != tt.wantNotFound || strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("got %v err %v, want %v notFound=%v", got, err, tt.want, tt.wantNotFound)
			}
		})
	}
}

func TestEditAppDomainsChangesOnlyDomains(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	var res editDomainsResponse
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"add":["A.example.com"]}`, &res); code != http.StatusOK {
		t.Fatalf("add = %d", code)
	}
	if !res.Changed || strings.Join(res.Domains, ",") != "a.example.com" {
		t.Fatalf("add result = %+v", res)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "nginx:1" || svc.Env["A"] != "1" || strings.Join(svc.Domains, ",") != "a.example.com" {
		t.Fatalf("stored = %+v", svc)
	}

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "other", Image: "x:1", Port: 80, Domains: []string{"taken.example.com"}}); err != nil {
		t.Fatal(err)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"add":["taken.example.com"]}`, nil); code != http.StatusConflict {
		t.Fatalf("taken domain = %d, want 409", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"remove":["nope.example.com"]}`, nil); code != http.StatusBadRequest {
		t.Fatalf("remove missing = %d, want 400", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"set":[],"add":["x.example.com"]}`, nil); code != http.StatusBadRequest {
		t.Fatalf("set with add = %d, want 400", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/missing/domains", `{"set":[]}`, nil); code != http.StatusNotFound {
		t.Fatalf("missing app = %d, want 404", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"set":[]}`, &res); code != http.StatusOK || !res.Changed || len(res.Domains) != 0 {
		t.Fatalf("set empty = %d %+v", code, res)
	}
}
