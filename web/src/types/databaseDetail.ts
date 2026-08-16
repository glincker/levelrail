// Wire type for the database resource, GET/POST /api/v1/databases and
// GET/DELETE /api/v1/databases/{name} (internal/api/databases.go's
// databaseResource). Field names mirror the JSON wire shape directly,
// the same snake_case-matches-wire-shape convention appDetail.ts
// documents: this is an internal admin dashboard talking to one frozen
// backend contract, not a public SDK, and a remapping layer would just
// be one more place for the two shapes to silently drift apart.
//
// node_id carries `omitempty` on the Go side and is response-only:
// databaseResource's own doc comment is explicit there is no
// create/edit affordance for it yet (no PUT /databases/{name}/node
// route), so this type exists to display it, never to send it back.

export type DatabaseEngine = 'postgres' | 'redis' | 'mysql'

export interface DatabaseResource {
  name: string
  engine: string
  version: string
  node_id?: string
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
}
