// Mirrors nodemove.RollingDrainOrder so the dialog previews the same order
// the CLI would use: fewest workloads first, refused below min_available.
export interface RollingNode {
  id: string
  online: boolean
  schedulable: boolean
  workloads: number
}

export type RollingOrderResult =
  | { ok: true; order: string[] }
  | { ok: false; reason: 'empty' | 'minAvailable'; stay: number }

export function rollingDrainOrder(
  group: RollingNode[],
  others: RollingNode[],
  minAvailable: number,
): RollingOrderResult {
  if (group.length === 0) {
    return { ok: false, reason: 'empty', stay: 0 }
  }
  const stay = others.filter((n) => n.online && n.schedulable).length
  if (stay < Math.max(minAvailable, 1)) {
    return { ok: false, reason: 'minAvailable', stay }
  }
  const order = [...group]
    .sort((a, b) => a.workloads - b.workloads)
    .map((n) => n.id)
  return { ok: true, order }
}
