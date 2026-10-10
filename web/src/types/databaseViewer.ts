// Wire shapes for the database viewer routes (internal/api/database_viewer*.go).

export interface DbColumn {
  name: string
  type: string
  nullable: boolean
  default?: string
  primary_key: boolean
}

export interface DbIndex {
  name: string
  definition: string
  unique: boolean
  primary: boolean
}

export interface DbTable {
  name: string
  kind: string
  row_estimate: number
  size_bytes: number
  columns: DbColumn[]
  indexes: DbIndex[]
}

export interface DbSchemaNode {
  name: string
  tables: DbTable[]
}

export interface DbViewerLimits {
  max_rows: number
  max_cell_bytes: number
  timeout_ms: number
}

export interface DbSchemaResponse {
  engine: string
  schemas: DbSchemaNode[]
  table_limit: number
  truncated: boolean
  limits: DbViewerLimits
}

export interface DbForeignKey {
  name: string
  schema: string
  table: string
  columns: string[]
  ref_schema: string
  ref_table: string
  ref_columns: string[]
}

export interface DbStructure {
  schema: string
  name: string
  kind: string
  row_estimate: number
  size_bytes: number
  columns: DbColumn[]
  indexes: DbIndex[]
  foreign_keys: DbForeignKey[]
  referenced_by: DbForeignKey[]
  ddl: string
}

/** A null cell is SQL NULL. */
export type DbCell = string | null

export interface DbResult {
  columns: string[]
  rows: DbCell[][]
  row_count: number
  truncated: boolean
  duration_ms: number
}

export interface DbQueryResponse extends DbResult {
  mode: 'read' | 'write' | 'explain'
  kind: string
  fingerprint: string
}

export interface DbPage extends DbResult {
  has_more: boolean
}

export type DbFilterOp = 'contains' | 'equals' | 'is_null' | 'not_null'

export interface DbColumnFilter {
  column: string
  op: DbFilterOp
  value: string
}

export interface DbPageParams {
  schema: string
  table: string
  limit: number
  offset: number
  sort?: string
  desc?: boolean
  filters?: DbColumnFilter[]
}

export interface DbQueryHistoryEntry {
  id: string
  mode: string
  sql: string
  fingerprint: string
  ok: boolean
  error?: string
  duration_ms: number
  row_count: number
  created_at: string
}

export interface DbSavedQuery {
  id: string
  name: string
  sql: string
  created_at: string
}

export interface RedisKey {
  key: string
  type: string
  ttl: number
}

export interface RedisScan {
  cursor: string
  keys: RedisKey[]
}

export interface RedisValue {
  type: string
  ttl: number
  lines: string[]
  truncated: boolean
}
