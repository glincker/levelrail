package probe_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/probe"
)

func ExampleConfig_Validate() {
	fmt.Println(probe.Config{Path: "/healthz", ExpectedStatus: "200-299"}.Validate())
	fmt.Println(probe.Config{Path: "healthz"}.Validate())
	// Output:
	// <nil>
	// path "healthz" must start with /
}

func ExampleProber_Check() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := probe.New(srv.Client(), nil, probe.Limits{})
	target := probe.Target{Addr: strings.TrimPrefix(srv.URL, "http://")}
	err := p.Check(context.Background(), target, probe.Config{Path: "/healthz", Timeout: 2 * time.Second})
	fmt.Println(err)
	// Output:
	// <nil>
}
