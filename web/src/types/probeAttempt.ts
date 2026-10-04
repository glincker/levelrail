// Wire type for GET /api/v1/apps/{name}/deploys/{deployId}/probes
// (internal/api/deploy_probes.go's handleListProbeAttempts,
// probeAttemptResource): one row per individual readiness-probe attempt
// a deploy's cutover made (internal/probe.WithOnAttempt), the per-attempt
// detail (status code, latency) ConditionsPanel's own single
// Reason/Message summary never captures. Empty for a deploy whose
// service has no readiness probe configured, or whose cutover was still
// starting when the container became ready on its first attempt.
export interface ProbeAttempt {
  id: number
  target: string
  success: boolean
  status_code?: number
  exit_code?: number
  error?: string
  latency_ms: number
  probed_at: string
}
