// Wire type for GET /api/v1/databases/{name}/slow-queries
// (internal/api/database_slow_queries.go's slowQueriesResponse/
// slowQueryEntryResource). rowsExamined is only ever set for a MySQL
// entry (Postgres' own slow-statement log line has no rows-examined
// count), so it's optional here rather than always a number.

export interface SlowQueryEntry {
  timestamp: string
  duration_ms: number
  query: string
  rows_examined?: number
}

export interface SlowQueriesResponse {
  entries: SlowQueryEntry[]
  total: number
}
