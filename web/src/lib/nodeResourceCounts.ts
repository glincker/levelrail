interface CountableNode {
  id: string
  is_local?: boolean
}

interface Placed {
  node_id?: string
}

// Apps and databases per node id. An empty node_id belongs to the local node.
export function countResourcesByNode(
  nodes: readonly CountableNode[],
  apps: readonly Placed[],
  databases: readonly Placed[],
): Map<string, number> {
  const localId = nodes.find((n) => n.is_local)?.id
  const counts = new Map<string, number>(nodes.map((n) => [n.id, 0]))
  for (const r of [...apps, ...databases]) {
    const id = r.node_id ? r.node_id : localId
    if (id !== undefined && counts.has(id)) {
      counts.set(id, (counts.get(id) ?? 0) + 1)
    }
  }
  return counts
}
