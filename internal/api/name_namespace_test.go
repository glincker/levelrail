package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedNamespaceDatabase(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.CreateDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "orders-db", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
}

func TestNameNamespace_AppNamedLikeDatabaseRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNamespaceDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"orders-db","image":"levelrail/x:1","port":4000,"secret_env":["POSTGRES_PASSWORD"]}`))
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("status = %d, want 4xx; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "orders-db"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("app row was written: %v", err)
	}
}

func TestNameNamespace_DatabaseNamedLikeAppRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"web","engine":"postgres","version":"16"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredDatabase(context.Background(), "web"); !errors.Is(err, store.ErrDatabaseNotFound) {
		t.Fatalf("database row was written: %v", err)
	}
}

func TestNameNamespace_ComposeDerivedNameCollidingWithDatabaseRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedNamespaceDatabase(t, db)

	yaml := "services:\n  db:\n    image: postgres:16\n    environment:\n      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}\n"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/orders/compose", yaml))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "orders-db"); !errors.Is(err, store.ErrServiceNotFound) {
		t.Fatalf("service row was written: %v", err)
	}
}

func TestNameNamespace_CloneOntoDatabaseNameRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	seedNamespaceDatabase(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"orders-db"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestNameNamespace_StoreGuardsEveryWriter(t *testing.T) {
	_, db := newTestRouter(t)
	ctx := context.Background()
	seedNamespaceDatabase(t, db)
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "orders-db", Image: "levelrail/x:1", Port: 1}); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("SaveDesiredService err = %v, want ErrNameTaken", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/x:1", Port: 1}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := db.CreateDesiredDatabase(ctx, store.DesiredDatabase{Name: "web", Engine: "postgres", Version: "16"}); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("CreateDesiredDatabase err = %v, want ErrNameTaken", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "web", Engine: "postgres", Version: "16"}); !errors.Is(err, store.ErrNameTaken) {
		t.Fatalf("SaveDesiredDatabase err = %v, want ErrNameTaken", err)
	}
}

func TestComposeDeployRacingAppCreateHasOneWinner(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for i := 0; i < 8; i++ {
		stack := fmt.Sprintf("race%d", i)
		derived := stack + "-web"
		start := make(chan struct{})
		var wg sync.WaitGroup
		var composeCode, createCode int
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+stack+"/compose", webOnlyComposeYAML))
			composeCode = rec.Code
		}()
		go func() {
			defer wg.Done()
			<-start
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"`+derived+`","image":"levelrail/x:1","port":4000}`))
			createCode = rec.Code
		}()
		close(start)
		wg.Wait()

		composeOK := composeCode == http.StatusOK
		createOK := createCode == http.StatusCreated || createCode == http.StatusOK
		// An admin caller may legitimately overwrite its own app, so both can succeed in create-then-compose order.
		if !composeOK && !createOK {
			t.Fatalf("%s: compose=%d create=%d, want at least one winner", derived, composeCode, createCode)
		}
		members, err := db.ListServicesByApp(context.Background(), stack)
		if err != nil {
			t.Fatalf("list members: %v", err)
		}
		if !composeOK && len(members) != 0 {
			t.Fatalf("%s: losing compose deploy wrote %d service(s)", derived, len(members))
		}
	}
}

func TestNameNamespace_ComposeStackNamedLikeDatabaseRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.CreateDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "pg1", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/pg1/compose", webOnlyComposeYAML))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if _, err := db.GetAppByName(context.Background(), "pg1"); err == nil {
		t.Fatal("app row was created for a stack named like a database")
	}
}
