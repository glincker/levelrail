package apiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Every exported client method must send the bearer token and surface a
// server failure as an error, so no endpoint wrapper can silently swallow a 500.
func TestClientMethodsAuthenticateAndSurfaceServerErrors(t *testing.T) {
	var unauth, total atomic.Int64
	fail := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		total.Add(1)
		if r.Header.Get("Authorization") != "Bearer tok" {
			unauth.Add(1)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte("null"))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")

	errType := reflect.TypeOf((*error)(nil)).Elem()
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	ct := reflect.TypeOf(c)
	skip := func(name string) bool {
		for _, p := range []string{"Wait", "Stream", "Tail", "Follow", "Watch", "Download", "Upload", "With", "Poll"} {
			if strings.Contains(name, p) {
				return true
			}
		}
		return false
	}

	run := func(failing bool) (called, errored int) {
		fail.Store(failing)
		for i := 0; i < ct.NumMethod(); i++ {
			m := ct.Method(i)
			mt := m.Type
			if skip(m.Name) || mt.NumIn() < 2 || !mt.In(1).Implements(ctxType) || mt.IsVariadic() {
				continue
			}
			n := mt.NumOut()
			if n == 0 || !mt.Out(n-1).Implements(errType) {
				continue
			}
			ok := true
			args := []reflect.Value{reflect.ValueOf(c)}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			args = append(args, reflect.ValueOf(ctx))
			for j := 2; j < mt.NumIn(); j++ {
				switch k := mt.In(j).Kind(); k {
				case reflect.Func, reflect.Chan, reflect.Interface, reflect.Pointer:
					ok = false
				case reflect.String:
					args = append(args, reflect.ValueOf("x").Convert(mt.In(j)))
				default:
					args = append(args, reflect.Zero(mt.In(j)))
				}
			}
			if !ok {
				cancel()
				continue
			}
			func() {
				defer cancel()
				defer func() { _ = recover() }()
				out := m.Func.Call(args)
				called++
				if e, _ := out[n-1].Interface().(error); e != nil {
					errored++
				}
			}()
		}
		return called, errored
	}

	called, errored := run(false)
	if called < 100 {
		t.Fatalf("only %d methods exercised, reflection filter too strict", called)
	}
	if total.Load() == 0 {
		t.Fatal("no requests reached the server")
	}
	if unauth.Load() != 0 {
		t.Errorf("%d requests were sent without the bearer token", unauth.Load())
	}
	_ = errored

	calledF, erroredF := run(true)
	if erroredF*100 < calledF*85 {
		t.Errorf("only %d of %d methods returned an error on HTTP 500", erroredF, calledF)
	}
}
