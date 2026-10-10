// Command fakecoolify serves the read-only fake Coolify API for local runs
// of the app import flow. It is development tooling and is not shipped.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport/fakecoolify"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	image := flag.String("searxng-image", "", "image for the searxng app, default the real one")
	port := flag.String("searxng-port", "", "exposed port of the searxng app, default 8080")
	redact := flag.Bool("redact-sensitive", false, "return secret values empty, like a token without read:sensitive")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		slog.Error("listen failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	fake := fakecoolify.New()
	fake.SearxngImage, fake.SearxngPort, fake.RedactSensitive = *image, *port, *redact
	fmt.Printf("fake coolify on http://%s token %s\n", ln.Addr(), fakecoolify.Token)
	srv := &http.Server{Handler: fake.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil {
		slog.Error("serve failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
