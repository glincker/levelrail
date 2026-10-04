// Wire types for GET /api/v1/network/topology
// (internal/api/network_topology.go's networkTopologyResponse): a
// read-only, whole-mesh summary for the /network topology page. Every
// fact here already exists elsewhere in the API (GET /apps, /databases,
// /nodes, /loadbalancers); this is just the joined, graph-shaped view of
// it, same snake_case-matches-wire-shape convention nodeDetail.ts
// documents.

export interface NetworkTopologyNode {
  id: string
  name: string
  region?: string
  status: string
  schedulable: boolean
  mesh_address?: string
  is_local: boolean
}

export interface NetworkTopologyApp {
  name: string
  // node_id is the raw, possibly-empty desired_services.node_id ("" means
  // the control plane's own node): see internal/store.DesiredService.NodeID's
  // own doc comment. Group by it directly; an empty string groups an
  // unplaced app with the local node's own zone.
  node_id: string
  domains?: string[]
  dns_name?: string
  mesh_address?: string
}

export interface NetworkTopologyDatabase {
  name: string
  engine: string
  node_id: string
  dns_name?: string
  mesh_address?: string
}

export interface NetworkTopologyLoadBalancer {
  service: string
  algorithm: string
}

export interface NetworkTopologyConnection {
  app: string
  database: string
  env_var?: string
}

export interface NetworkTopologyResponse {
  zone: string
  mesh_enabled: boolean
  nodes: NetworkTopologyNode[]
  apps: NetworkTopologyApp[]
  databases: NetworkTopologyDatabase[]
  load_balancers: NetworkTopologyLoadBalancer[]
  connections: NetworkTopologyConnection[]
}
