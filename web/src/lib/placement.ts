export type PlacementKind = 'local' | 'remote' | 'unknown'

export interface PlacementInput {
  nodeId?: string
  nodeName?: string
  nodeIsLocal?: boolean
}

export interface PlacementNode {
  id: string
  name: string
  is_local?: boolean
}

export interface ResolvedPlacement {
  kind: PlacementKind
  name: string
}

// An empty or local node id means the control plane's own node; only an id
// that matches no known node is unknown. Server-resolved fields win, the
// node list covers older responses that carry only node_id.
export function resolvePlacement(
  input: PlacementInput,
  nodes: readonly PlacementNode[] = [],
): ResolvedPlacement {
  if (input.nodeIsLocal || !input.nodeId) {
    return { kind: 'local', name: input.nodeName ?? '' }
  }
  const known = nodes.find((n) => n.id === input.nodeId)
  if (known?.is_local) {
    return { kind: 'local', name: known.name }
  }
  const name = input.nodeName || known?.name
  if (name) {
    return { kind: 'remote', name }
  }
  return { kind: 'unknown', name: input.nodeId }
}
