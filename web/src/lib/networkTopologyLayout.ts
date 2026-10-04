// Pure grouping/derivation logic for the /network topology page
// (web/src/routes/network/index.tsx), split out from the rendering
// components so it's cheap to unit test without mounting anything.
//
// The mental model (per the AWS/Azure VPC console research this page is
// based on): the whole mesh is one flat "VPC", and each physical node is a
// "zone" grouping its own resources, labeled by its operator-set region
// (falling back to the node's own name when no region is set).

import type {
  NetworkTopologyApp,
  NetworkTopologyDatabase,
  NetworkTopologyLoadBalancer,
  NetworkTopologyNode,
  NetworkTopologyResponse,
} from '../types/networkTopology'

export interface TopologyZone {
  /** The real node ID, or "" for the synthetic "unplaced" zone. */
  key: string
  label: string
  node?: NetworkTopologyNode
  apps: NetworkTopologyApp[]
  databases: NetworkTopologyDatabase[]
  /** Keyed by the app/service name each balancer fronts. */
  loadBalancerByApp: Map<string, NetworkTopologyLoadBalancer>
}

const UNPLACED_ZONE_KEY = '__unplaced__'

/**
 * resolveZoneKey turns a placement's raw node_id (possibly "", meaning
 * the control plane's own node, per DesiredService.NodeID's own
 * contract) into the zone key to group it under: the real node ID when
 * one resolves, or UNPLACED_ZONE_KEY when it points at a node this
 * response doesn't know about at all (deleted node, or no nodes exist
 * yet), which is itself a real, worth-surfacing state rather than a bug
 * to hide.
 */
export function resolveZoneKey(
  nodeId: string,
  nodes: NetworkTopologyNode[],
): string {
  const id = nodeId || (nodes.find((n) => n.is_local)?.id ?? '')
  if (id && nodes.some((n) => n.id === id)) {
    return id
  }
  return UNPLACED_ZONE_KEY
}

export function zoneLabel(node: NetworkTopologyNode): string {
  return node.region?.trim() || node.name
}

/** groupIntoZones is the page's one data-shaping pass: one zone per node,
 * plus a trailing "Unplaced" zone only when something genuinely has
 * nowhere real to go. */
export function groupIntoZones(
  topology: NetworkTopologyResponse,
): TopologyZone[] {
  const zones = new Map<string, TopologyZone>()
  for (const node of topology.nodes) {
    zones.set(node.id, {
      key: node.id,
      label: zoneLabel(node),
      node,
      apps: [],
      databases: [],
      loadBalancerByApp: new Map(),
    })
  }

  const unplaced: TopologyZone = {
    key: UNPLACED_ZONE_KEY,
    label: 'Unplaced',
    apps: [],
    databases: [],
    loadBalancerByApp: new Map(),
  }
  const zoneFor = (nodeId: string): TopologyZone => {
    const key = resolveZoneKey(nodeId, topology.nodes)
    return zones.get(key) ?? unplaced
  }

  for (const app of topology.apps) {
    zoneFor(app.node_id).apps.push(app)
  }
  for (const database of topology.databases) {
    zoneFor(database.node_id).databases.push(database)
  }
  const lbByService = new Map(
    topology.load_balancers.map((lb) => [lb.service, lb]),
  )
  for (const app of topology.apps) {
    const lb = lbByService.get(app.name)
    if (lb) {
      zoneFor(app.node_id).loadBalancerByApp.set(app.name, lb)
    }
  }

  const out = [...zones.values()]
  if (
    unplaced.apps.length > 0 ||
    unplaced.databases.length > 0 ||
    unplaced.loadBalancerByApp.size > 0
  ) {
    out.push(unplaced)
  }
  return out
}

/** appHasMeshAddress: whether an app/database resource resolves to a
 * real mesh address right now, the concrete "is this reachable" signal
 * the page surfaces per-resource rather than only per-node. */
export function hasMeshAddress(entry: { mesh_address?: string }): boolean {
  return Boolean(entry.mesh_address)
}
