import type { NodeProviderSizeResource } from '../types/nodeProvision'

export interface OracleFlexPreset {
  id: string
  ocpus: number
  memoryGb: number
}

export interface OracleFlexShape {
  shape: string
  presets: OracleFlexPreset[]
}

// Flex shapes come back as "Shape:ocpus:memoryGB" size ids, fixed shapes as
// the bare shape name (internal/provision/oracle.go parseOracleSize).
export function splitOracleSizes(sizes: NodeProviderSizeResource[]): {
  fixed: NodeProviderSizeResource[]
  flex: OracleFlexShape[]
} {
  const fixed: NodeProviderSizeResource[] = []
  const byShape = new Map<string, OracleFlexPreset[]>()
  for (const s of sizes) {
    const [shape, ocRaw, memRaw, ...rest] = s.id.split(':')
    if (
      shape === undefined ||
      ocRaw === undefined ||
      memRaw === undefined ||
      rest.length > 0
    ) {
      fixed.push(s)
      continue
    }
    const ocpus = Number(ocRaw)
    const memoryGb = Number(memRaw)
    if (!(ocpus > 0) || !(memoryGb > 0)) {
      fixed.push(s)
      continue
    }
    const list = byShape.get(shape) ?? []
    list.push({ id: s.id, ocpus, memoryGb })
    byShape.set(shape, list)
  }
  const flex = [...byShape.entries()].map(([shape, presets]) => ({
    shape,
    presets,
  }))
  return { fixed, flex }
}

export function oracleFlexId(
  shape: string,
  ocpus: number,
  memoryGb: number,
): string {
  return `${shape}:${ocpus}:${memoryGb}`
}

export function oracleFlexShapeOf(id: string): string | null {
  const parts = id.split(':')
  return parts.length === 3 ? (parts[0] ?? null) : null
}

export type OracleGuardKind =
  'enabled' | 'a1' | 'micro' | 'paidShape' | 'unknownPrice'

// Maps a refusal message from the create guard to the setting it names, so
// the UI can say what to change instead of only echoing the server text.
export function oracleGuardKind(message: string): OracleGuardKind | null {
  if (message.includes('provision.oracle.enabled')) return 'enabled'
  if (message.includes('provision.oracle.max_a1_')) return 'a1'
  if (message.includes('provision.oracle.max_e2_micro')) return 'micro'
  if (message.includes('provision.oracle.allow_paid_shapes')) return 'paidShape'
  if (message.includes('price cap cannot be checked')) return 'unknownPrice'
  return null
}
