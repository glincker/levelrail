// Wire type for a node, GET /api/v1/nodes and GET /api/v1/nodes/{id}
// (internal/api/nodes.go's nodeResource). Same snake_case-matches-wire-
// shape convention appDetail.ts and databaseDetail.ts document: an
// internal admin dashboard talking to one frozen backend contract, not
// a public SDK.

// 'cordoned' is kept here purely because it's a real value NodeStatus's
// Go type (internal/store/nodes.go's NodeStatus) can hold, not because
// anything currently produces it: that file's own doc comment on
// NodeStatusCordoned says it is unused as of TASKS.md 3.7, and the only
// two real callers of UpdateNodeStatus (internal/reconcile/nodehealth's
// controller and internal/agent/server.go's heartbeat handling) only
// ever write NodeStatusOnline or NodeStatusOffline. Cordon's real,
// currently-live signal is the separate `schedulable` field below
// (nodeResource's own doc comment in internal/api/nodes.go is explicit
// that Schedulable is "a separate field from Status rather than folded
// into it"), so UI code must drive cordon/uncordon display off
// `schedulable`, not off `status === 'cordoned'`, which nothing sets.
export type NodeStatus = 'pending' | 'online' | 'offline' | 'cordoned'

export interface NodeResource {
  id: string
  name: string
  address?: string
  status: NodeStatus
  cert_fingerprint?: string
  joined_at?: string
  last_seen_at?: string
  // Schedulable is cordon's real backing state (see the NodeStatus
  // comment above): false means the node refuses new placements but
  // keeps running whatever's already on it. Toggled by
  // POST /api/v1/nodes/{id}/cordon and /uncordon.
  schedulable: boolean
  // AcceptsAppWorkloads/AcceptsBuildWorkloads are independent capability
  // flags (internal/store.Node's own doc comment: "a node can be either,
  // both, or neither"), full-replaced together via
  // PUT /api/v1/nodes/{id}/workloads, never patched individually.
  accepts_app_workloads: boolean
  accepts_build_workloads: boolean
  created_at: string
  // AlertStatus is only present on GET /api/v1/nodes/{id} (the single-
  // node fetch), never the list response, and is absent when telemetry
  // isn't configured on this control plane. It's a live re-evaluation of
  // this node's own standing for each node-scoped, platform-wide alert
  // kind, not a rule's stored aggregate value (which only ever names the
  // worst node across the whole fleet, never which one).
  alert_status?: NodeAlertStatusResource
}

// Each field is 'ok' (no recent breach), 'firing' (this node is over
// threshold right now), or 'unknown' (no recent sample to judge, e.g. the
// collector hasn't run yet): 'unknown' must never be treated the same as
// 'ok' by UI code, the same "don't silently confirm what you can't see"
// stance the backend evaluators already take.
export type NodeAlertState = 'ok' | 'firing' | 'unknown'

export interface NodeAlertStatusResource {
  patch_status: NodeAlertState
  node_disk_space: NodeAlertState
  node_resource_usage: NodeAlertState
}

// Response body for POST /api/v1/nodes/join-tokens
// (internal/api/nodes.go's createNodeJoinTokenResponse): the token's one
// and only appearance in plaintext, the same "shown once" shape a
// created API token already has (see queries/auth.ts's own token
// handling).
export interface NodeJoinTokenResponse {
  token: string
  expires_at: string
}

// Response body for POST /api/v1/nodes/{id}/drain
// (internal/api/nodes.go's drainNodeResponse). A partial failure is not
// an error response: status is 200 if Errors is empty, 207 Multi-Status
// if some resources failed to move, but everything not named in Errors
// still moved successfully, per that handler's own doc comment. UI code
// must render Errors alongside MovedServices/MovedDatabases rather than
// treating the whole request as pass/fail.
export interface DrainNodeResponse {
  target_node_id: string
  moved_services: string[]
  moved_databases: string[]
  errors?: string[]
}

// Response body for GET /api/v1/nodes/{id}/patch-status
// (internal/api/node_patch_status.go's nodePatchStatusResponse): the
// latest OS-patch reading HostPatchCollector wrote for this node.
// `checked` distinguishes "never checked" (no supported package manager,
// or the collector hasn't run yet) from a checked, genuinely empty
// `total`; UI code must branch on `checked` first, never infer "unknown"
// from `total === 0`.
export interface NodePatchStatusResource {
  checked: boolean
  total: number
  security: number
  checked_at?: string
}
