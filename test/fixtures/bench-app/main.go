// Command bench-app is the fixture HTTP app the load harness deploys.
package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	host, _ := os.Hostname()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Replica", host)
		_, _ = w.Write([]byte("ok"))
	})
	// The container exits so the harness can fail one replica without host access.
	mux.HandleFunc("POST /__bench/die", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		go func() {
			time.Sleep(50 * time.Millisecond)
			os.Exit(1)
		}()
	})
	srv := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
