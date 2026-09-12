// Wire type for the database resource, GET/POST /api/v1/databases and
// GET/DELETE /api/v1/databases/{name} (internal/api/databases.go's
// databaseResource). Field names mirror the JSON wire shape directly,
// the same snake_case-matches-wire-shape convention appDetail.ts
// documents: this is an internal admin dashboard talking to one frozen
// backend contract, not a public SDK, and a remapping layer would just
// be one more place for the two shapes to silently drift apart.
//
// node_id carries `omitempty` on the Go side and is response-only on
// update: databaseResource's own doc comment is explicit an existing
// database's placement changes via PUT /api/v1/databases/{name}/node
// (handleSetDatabaseNode, useSetDatabaseNode) rather than through this
// type. POST /api/v1/databases (create) is the one exception: an
// explicit node_id there overrides simple spread scheduling, see
// CreateDatabaseRequest's own doc comment (queries/databases.ts).

import type { AppStatusSummary, ServiceResources } from './appDetail'
export type { ServiceResources } from './appDetail'

export type DatabaseEngine = 'postgres' | 'redis' | 'mysql'

export interface DatabaseResource {
  name: string
  engine: string
  version: string
  node_id?: string
  // auto_placed carries `omitempty` on the Go side and is response-only,
  // set only by POST /api/v1/databases: true when the create request
  // omitted node_id and simple spread scheduling picked a non-local node
  // for it.
  auto_placed?: boolean
  // project_id: response-only on PUT, the same shape appDetail.ts
  // documents for AppDetail's own project_id, set via
  // PUT /api/v1/databases/{name}/project (useSetDatabaseProject) or, at
  // create time only, POST /api/v1/databases's request body directly.
  project_id?: string
  // publicly_accessible, public_port: response-only, the identical
  // boundary node_id/project_id already establish. Set or clear via
  // PUT/DELETE /api/v1/databases/{name}/public-access
  // (useSetDatabasePublicAccess/useClearDatabasePublicAccess,
  // queries/databases.ts). public_port carries `omitempty` on the Go
  // side (internal/api/databases.go's databaseResource), so it's absent
  // entirely, not 0, whenever publicly_accessible is false.
  publicly_accessible?: boolean
  public_port?: number
  // resources: internal/store.ServiceResources reused verbatim by
  // DesiredDatabase (same as DesiredService), so this imports
  // appDetail.ts's ServiceResources rather than redeclaring an identical
  // shape. Set via PUT /api/v1/databases/{name}/resources
  // (useSetDatabaseResources): databases have no general
  // PUT /databases/{name} the way AppDetail.resources gets set through
  // handleUpdateApp, so this field is settable only through its own
  // dedicated route.
  resources?: ServiceResources | null
  // resources_applied_live: see appDetail.ts's identically-named field
  // doc comment. Only ever set by PUT /api/v1/databases/{name}/resources.
  resources_applied_live?: boolean
  // backup_target_id/backup_schedule/backup_retain: response-only, the
  // same boundary publicly_accessible/public_port already establish. Set
  // or cleared via PUT/DELETE /api/v1/databases/{name}/backup-schedule
  // (useSetDatabaseBackupSchedule/useClearDatabaseBackupSchedule,
  // queries/databases.ts).
  backup_target_id?: string
  backup_schedule?: string
  backup_retain?: number
  backup_retain_days?: number
}

// GET /api/v1/databases' own wire shape (internal/api/databases.go's
// databaseListResource): DatabaseResource plus a batched status summary
// computed from one query across every listed database's conditions,
// not present on the detail endpoint's DatabaseResource. Deliberately
// its own type, mirroring AppListEntry's exact reasoning (appDetail.ts):
// GET /api/v1/databases/{name} never sends `status`.
export interface DatabaseListEntry extends DatabaseResource {
  status: AppStatusSummary
}
