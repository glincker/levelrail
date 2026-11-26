import type { NodeResource } from '../../types/nodeDetail'
import type { NodeResourceUsage } from '../../types/fleetResourceUsage'

export type PlacementKind = 'local' | 'node' | 'cloud' | 'host'

export const AUTO_NODE = '__auto__'

const DEFAULT_MIN_FREE_DISK_PERCENT = 15

// Mirrors the backend placement floor; the server stays the authority.
export function minFreeDiskPercent(): number {
  const raw = Number(
    import.meta.env.VITE_DATABASE_PLACEMENT_MIN_FREE_DISK_PERCENT ??
      DEFAULT_MIN_FREE_DISK_PERCENT,
  )
  return Number.isFinite(raw) && raw >= 0 && raw < 100
    ? raw
    : DEFAULT_MIN_FREE_DISK_PERCENT
}

export type IneligibleReason =
  'offline' | 'cordoned' | 'outdated' | 'failing' | 'noDocker' | 'lowDisk'

export interface PlacementNode {
  id: string
  name: string
  isLocal: boolean
  online: boolean
  schedulable: boolean
  outdated: boolean
  failing: boolean
  // undefined means the node does not report it, never "absent".
  docker?: boolean
  diskFreePercent?: number
  diskFreeBytes?: number
  cpuPercent?: number
  workloads: number
  zone?: string
  region?: string
}

export function toPlacementNode(
  node: NodeResource,
  usage?: NodeResourceUsage,
): PlacementNode {
  const total = usage?.disk_total_bytes
  const used = usage?.disk_used_bytes
  const hasDisk = total !== undefined && used !== undefined && total > 0
  return {
    id: node.id,
    name: node.name,
    isLocal: node.is_local,
    online: node.status === 'online',
    schedulable: node.schedulable,
    outdated: node.agent?.outdated === true,
    failing: node.failure !== undefined,
    diskFreeBytes: hasDisk ? total - used : undefined,
    diskFreePercent: hasDisk ? ((total - used) / total) * 100 : undefined,
    cpuPercent: usage?.cpu_percent,
    workloads: node.resource_count ?? 0,
    zone: node.zone,
    region: node.region,
  }
}

export function ineligibleReason(
  node: PlacementNode,
  minFree: number = minFreeDiskPercent(),
): IneligibleReason | null {
  if (!node.online) return 'offline'
  if (!node.schedulable) return 'cordoned'
  if (node.failing) return 'failing'
  if (node.outdated) return 'outdated'
  if (node.docker === false) return 'noDocker'
  if (node.diskFreePercent !== undefined && node.diskFreePercent < minFree) {
    return 'lowDisk'
  }
  return null
}

// Known values rank ahead of unknown ones so a missing reading never wins.
function compareKnown(
  a: number | undefined,
  b: number | undefined,
  dir: 1 | -1,
) {
  if (a === b) return 0
  if (a === undefined) return 1
  if (b === undefined) return -1
  return (a - b) * dir
}

// Healthiest first: lowest load, then most free disk, then fewest
// workloads, then name and id so the result never depends on list order.
export function pickAutoNode(
  nodes: readonly PlacementNode[],
  minFree: number = minFreeDiskPercent(),
): PlacementNode | null {
  const eligible = nodes.filter((n) => ineligibleReason(n, minFree) === null)
  if (eligible.length === 0) return null
  const sorted = [...eligible].sort(
    (a, b) =>
      compareKnown(a.cpuPercent, b.cpuPercent, 1) ||
      compareKnown(a.diskFreePercent, b.diskFreePercent, -1) ||
      a.workloads - b.workloads ||
      a.name.localeCompare(b.name) ||
      a.id.localeCompare(b.id),
  )
  return sorted[0] ?? null
}

// The node id the create request carries, undefined lets the server place it.
export function resolvePlacementNodeId(
  kind: PlacementKind,
  nodeId: string,
  nodes: readonly PlacementNode[],
): string | undefined {
  if (kind === 'local') return nodes.find((n) => n.isLocal)?.id
  if (kind !== 'node') return undefined
  if (nodeId === AUTO_NODE) return pickAutoNode(nodes)?.id
  return nodeId
}

// Cloud and host choices need a node that does not exist yet.
export function placementBlocksCreate(kind: PlacementKind): boolean {
  return kind === 'cloud' || kind === 'host'
}
