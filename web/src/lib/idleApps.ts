import type { AdvisorFinding } from '../types/resourceAdvisor'

export function idleAppFindings(findings: AdvisorFinding[]): AdvisorFinding[] {
  return findings
    .filter((f) => f.kind === 'idle_app' && f.resource.kind === 'app')
    .sort((a, b) => (b.monthly_saving_usd ?? 0) - (a.monthly_saving_usd ?? 0))
}
