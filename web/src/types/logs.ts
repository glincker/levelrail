// Wire type for GET /api/v1/apps/{name}/logs (internal/api/logs.go's
// logsResponse/logEntryResource). `fields` mirrors that handler's
// `FieldsJSON` verbatim: present only when `structured` is true, and
// already-parsed JSON (an object, per logs.go's own doc comment
// restricting structured detection to JSON objects specifically), not a
// raw string LogSearchPanel needs to re-parse.

export interface LogEntry {
  timestamp: string
  stream: 'stdout' | 'stderr'
  message: string
  structured: boolean
  fields?: Record<string, unknown>
  // Detected level (trace, debug, info, warn, error, fatal), absent when
  // the line carries none.
  level?: string
}

export interface LogsResponse {
  entries: LogEntry[]
  // Entries that matched the time window and query before `limit` trimmed
  // them to the newest N.
  total?: number
}

export interface LogsResult {
  entries: LogEntry[]
  total: number
}
