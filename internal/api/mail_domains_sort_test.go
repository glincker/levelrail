package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMailDomains_ListSortsByDomain(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.mail.boxes = db
	cookie := loginTestSession(t, rt, db)
	for _, d := range []string{"b.example.test", "a.example.test", "c.example.test"} {
		if err := db.ClaimMailDomain(context.Background(), store.MailDomain{Domain: d, Service: "mail-mox", TeamID: "team-a", CreatedBy: "u"}); err != nil {
			t.Fatal(err)
		}
	}
	for query, want := range map[string]string{"sort=domain": "a,b,c", "sort=-domain": "c,b,a"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/mail/domains?"+query, ""))
		var list []mailDomainResource
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil {
			t.Fatalf("%s: %d %s", query, rec.Code, rec.Body.String())
		}
		got := make([]string, 0, len(list))
		for _, d := range list {
			got = append(got, strings.TrimSuffix(d.Domain, ".example.test"))
		}
		if strings.Join(got, ",") != want {
			t.Fatalf("%s: got %v, want %s", query, got, want)
		}
	}
}
