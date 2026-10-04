// Wire types for GET /api/v1/projects/{id}/topology
// (internal/api/topology.go's handleGetProjectTopology /
// internal/api/topology_graph.go's buildTopologyGraph): a project-scoped
// diagram of how its apps, databases, and shared volumes actually
// relate. Unlike GET /api/v1/network/topology (types/networkTopology.ts),
// which groups by node and only knows about app-to-database
// connections, this is grouped by resource kind and carries every edge
// kind buildTopologyGraph derives: database bindings, depends_on,
// volume attachments, and egress allow-rules that resolve to another
// app in the same project.

export type ProjectTopologyNodeKind = 'app' | 'database' | 'volume'

export type ProjectTopologyEdgeKind =
  'database_binding' | 'depends_on' | 'volume_attachment' | 'egress_allow'

// Mirrors internal/api/app_status.go's appStatusSummary exactly, the
// same wire shape GET /api/v1/apps and GET /api/v1/databases already
// use for their own list rows (types/appDetail.ts's AppStatusSummary):
// one status convention, reused here rather than invented again.
export interface ProjectTopologyStatus {
  label: string
  variant: 'muted' | 'destructive' | 'success'
}

export interface ProjectTopologyNode {
  id: string
  kind: ProjectTopologyNodeKind
  label: string
  status?: ProjectTopologyStatus
}

export interface ProjectTopologyEdge {
  from: string
  to: string
  kind: ProjectTopologyEdgeKind
}

export interface ProjectTopologyGraph {
  nodes: ProjectTopologyNode[]
  edges: ProjectTopologyEdge[]
}
