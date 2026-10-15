/** AccessRecord mirrors internal/ingress's AccessRecord. */
export interface AccessRecord {
  time: string
  host: string
  method: string
  path: string
  status: number
  duration_ms: number
  bytes_in: number
  bytes_out: number
  request_id: string
  trace_id?: string
  upstream_error?: boolean
}
