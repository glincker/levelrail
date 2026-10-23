package api

import "net/http"

func (rt *Router) registerChaosRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/resilience", rt.requireAbility(AbilityRead, rt.handleChaosReport))
	mux.HandleFunc("GET /api/v1/resilience/disk", rt.requireAbility(AbilityRead, rt.handleGetDiskResilience))
	mux.HandleFunc("POST /api/v1/resilience/disk/free-space", rt.requireAbility(AbilityRoot, rt.handleFreeDiskSpace))
	mux.HandleFunc("POST /api/v1/resilience/results", rt.requireAbility(AbilityWrite, rt.handleRecordChaosResults))
	mux.HandleFunc("GET /api/v1/benchmarks", rt.requireAbility(AbilityRead, rt.handleListBenchmarks))
	mux.HandleFunc("POST /api/v1/benchmarks", rt.requireAbility(AbilityWrite, rt.handleUploadBenchmark))
	mux.HandleFunc("GET /api/v1/benchmarks/compare", rt.requireAbility(AbilityRead, rt.handleCompareBenchmarks))
	mux.HandleFunc("GET /api/v1/benchmarks/{id}", rt.requireAbility(AbilityRead, rt.handleGetBenchmark))
	mux.HandleFunc("DELETE /api/v1/benchmarks/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteBenchmark))
}
