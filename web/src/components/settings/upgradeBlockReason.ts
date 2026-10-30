import type { SelfUpgradePlan } from '../../queries/selfUpgrade'

export type BlockReason =
  | { kind: 'disk'; shortBytes: number }
  | { kind: 'plan'; reason: string }
  | { kind: 'notes' }

// Why the Upgrade button must stay disabled, or null when it may be pressed.
export function upgradeBlockReason(plan: SelfUpgradePlan): BlockReason | null {
  if (!plan.can_apply) {
    return { kind: 'plan', reason: plan.cannot_apply_reason ?? '' }
  }
  if (!plan.notes_available) return { kind: 'notes' }
  const d = plan.disk
  if (d && d.known && !d.ok) {
    return {
      kind: 'disk',
      shortBytes: Math.max(0, d.required_bytes - d.free_bytes),
    }
  }
  return null
}
