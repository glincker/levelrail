import type { AbuseMode, AbusePolicy, AppAbuse } from '../../types/abuse'

/** AbuseDraft is the form state: numbers and lists stay text until they are sent. */
export interface AbuseDraft {
  mode: AbuseMode
  rps: string
  burst: string
  cidrs: string
  agents: string
  requireUa: boolean
}

export function parseList(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

/**
 * draftFrom never starts blank: an app with no policy begins in log_only with
 * the rate suggested from its recent peak, so saving counts and blocks nothing.
 */
export function draftFrom(a: AppAbuse): AbuseDraft {
  const p = a.policy
  const unset = !a.configured
  const rps = unset || !p.rps ? a.suggestion.rps : p.rps
  const burst = unset || !p.burst ? a.suggestion.burst : p.burst
  return {
    mode: unset ? 'log_only' : p.mode,
    rps: String(rps),
    burst: String(burst),
    cidrs: (p.deny_cidrs ?? []).join('\n'),
    agents: (p.deny_user_agents ?? []).join('\n'),
    requireUa: p.require_user_agent ?? false,
  }
}

function parseNonNegative(text: string): number | null {
  if (text.trim() === '') return 0
  const n = Number(text)
  return Number.isFinite(n) && n >= 0 ? n : null
}

/** draftToPolicy returns null while the rate or burst is not a number. */
export function draftToPolicy(d: AbuseDraft): AbusePolicy | null {
  const rps = parseNonNegative(d.rps)
  const burst = parseNonNegative(d.burst)
  if (rps === null || burst === null) return null
  const p: AbusePolicy = { mode: d.mode }
  if (rps > 0) {
    p.rps = rps
    if (burst > 0) p.burst = Math.floor(burst)
  }
  const cidrs = parseList(d.cidrs)
  const agents = parseList(d.agents)
  if (cidrs.length > 0) p.deny_cidrs = cidrs
  if (agents.length > 0) p.deny_user_agents = agents
  if (d.requireUa) p.require_user_agent = true
  return p
}

export function hasProtection(p: AbusePolicy): boolean {
  return (
    (p.rps ?? 0) > 0 ||
    (p.deny_cidrs?.length ?? 0) > 0 ||
    (p.deny_user_agents?.length ?? 0) > 0 ||
    p.require_user_agent === true
  )
}

/** policyKey is a stable string for comparing two policies. */
export function policyKey(p: AbusePolicy): string {
  return JSON.stringify([
    p.mode,
    p.rps ?? 0,
    p.burst ?? 0,
    p.deny_cidrs ?? [],
    p.deny_user_agents ?? [],
    p.require_user_agent ?? false,
  ])
}
