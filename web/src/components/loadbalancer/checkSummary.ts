import type { CheckSample } from '../../queries/loadBalancerLive'

export const BOXES = 12

export function summarizeChecks(checks: CheckSample[]): string {
  const last = checks.slice(-BOXES)
  const passed = last.filter((c) => c.ok).length
  return `Last ${last.length} checks: ${passed} passed, ${last.length - passed} failed`
}
