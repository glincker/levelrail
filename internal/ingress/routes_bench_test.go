package ingress

import (
	"encoding/json"
	"fmt"
	"testing"
)

func benchProxyRoutes(n int) []ProxyRoute {
	routes := make([]ProxyRoute, n)
	for i := range routes {
		routes[i] = ProxyRoute{
			Hosts:       []string{fmt.Sprintf("app-%d.example.test", i), fmt.Sprintf("www.app-%d.example.test", i)},
			BackendDial: fmt.Sprintf("127.0.0.1:%d", 20000+i),
		}
	}
	return routes
}

func benchRoutesOptions(n int) RoutesOptions {
	return RoutesOptions{
		ServerName:  "bench",
		ListenAddr:  ":443",
		TLS:         true,
		ACMEEnabled: true,
		ACMEEmail:   "ops@example.test",
		Routes:      benchProxyRoutes(n),
	}
}

// BenchmarkBuildRoutesConfig measures the full rebuild every ingress pass pays.
func BenchmarkBuildRoutesConfig(b *testing.B) {
	for _, n := range []int{10, 100, 500, 2000} {
		b.Run(fmt.Sprintf("routes=%d", n), func(b *testing.B) {
			opts := benchRoutesOptions(n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := BuildRoutesConfig(opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBuildAndMarshalRoutesConfig adds the JSON marshal Driver.Apply does to detect an unchanged config.
func BenchmarkBuildAndMarshalRoutesConfig(b *testing.B) {
	for _, n := range []int{10, 100, 500, 2000} {
		b.Run(fmt.Sprintf("routes=%d", n), func(b *testing.B) {
			opts := benchRoutesOptions(n)
			b.ReportAllocs()
			b.ResetTimer()
			var size int
			for i := 0; i < b.N; i++ {
				cfg, err := BuildRoutesConfig(opts)
				if err != nil {
					b.Fatal(err)
				}
				payload, err := json.Marshal(cfg)
				if err != nil {
					b.Fatal(err)
				}
				size = len(payload)
			}
			b.ReportMetric(float64(size), "json-bytes")
		})
	}
}
