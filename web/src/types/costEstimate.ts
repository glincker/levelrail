// Wire type for GET /api/v1/apps/{name}/cost-estimate
// (internal/api/cost_estimate.go's costEstimateResource): a read-only,
// deterministic "what this would cost elsewhere" estimate derived from
// the app's declared or observed CPU/memory (internal/costestimate).
// This is an ESTIMATE, not a real bill: note always restates that.
export type CostBasis = 'declared' | 'observed' | 'unavailable'

export interface CostEstimateProvider {
  key: string
  label: string
  cpu_cost_usd: number
  memory_cost_usd: number
  total_usd: number
  minimum_applied: boolean
}

export interface CostEstimate {
  service_name: string
  vcpu_cores: number
  memory_gib: number
  cpu_basis: CostBasis
  memory_basis: CostBasis
  providers: CostEstimateProvider[]
  note: string
}
