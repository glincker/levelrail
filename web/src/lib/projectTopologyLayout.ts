// Pure grouping logic for the project topology page
// (web/src/routes/projects/$id/topology.tsx), split out from the
// rendering component so it's cheap to unit test without mounting
// anything, the same split lib/networkTopologyLayout.ts already
// establishes for the /network page.
//
// The mental model: three columns, one per node kind (apps, databases,
// volumes), each stacked top to bottom. ProjectTopologyView draws
// connector lines between whatever anchors land where after that
// ordinary flex layout; this file only decides which column a node goes
// in and in what order.

import type {
  ProjectTopologyEdge,
  ProjectTopologyGraph,
  ProjectTopologyNode,
  ProjectTopologyNodeKind,
} from '../types/projectTopology'

export interface TopologyColumn {
  kind: ProjectTopologyNodeKind
  title: string
  nodes: ProjectTopologyNode[]
}

const COLUMN_ORDER: { kind: ProjectTopologyNodeKind; title: string }[] = [
  { kind: 'app', title: 'Apps' },
  { kind: 'database', title: 'Databases' },
  { kind: 'volume', title: 'Volumes' },
]

// groupIntoColumns buckets every node by kind, in COLUMN_ORDER, dropping
// a column entirely when it has nothing: an empty project never renders
// three empty boxes.
export function groupIntoColumns(
  graph: ProjectTopologyGraph,
): TopologyColumn[] {
  const byKind = new Map<ProjectTopologyNodeKind, ProjectTopologyNode[]>()
  for (const node of graph.nodes) {
    const list = byKind.get(node.kind)
    if (list) {
      list.push(node)
    } else {
      byKind.set(node.kind, [node])
    }
  }
  return COLUMN_ORDER.flatMap(({ kind, title }) => {
    const nodes = byKind.get(kind)
    return nodes && nodes.length > 0 ? [{ kind, title, nodes }] : []
  })
}

// EDGE_LABEL and EDGE_DASH back the legend and the connector lines
// themselves: one visual language per edge kind, not per instance.
export const EDGE_LABEL: Record<ProjectTopologyEdge['kind'], string> = {
  database_binding: 'Database binding',
  depends_on: 'Depends on',
  volume_attachment: 'Volume attachment',
  egress_allow: 'Egress allow-rule',
}

export const EDGE_DASH: Record<ProjectTopologyEdge['kind'], string> = {
  database_binding: '0',
  depends_on: '4 3',
  volume_attachment: '0',
  egress_allow: '2 3',
}
