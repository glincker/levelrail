import { ApiError } from './apiError'

export type ExplorerErrorReason =
  | 'permission'
  | 'connection'
  | 'timeout'
  | 'notRunning'
  | 'helper'
  | 'network'
  | 'unsupported'

export interface ExplorerErrorInfo {
  reason: ExplorerErrorReason
  raw: string
}

const PATTERNS: [RegExp, ExplorerErrorReason][] = [
  [/permission denied|must be (owner|superuser)|access denied/i, 'permission'],
  [
    /connection refused|could not connect|not accepting connections|is starting up|no such file or directory.*socket|can't connect/i,
    'connection',
  ],
  [/timed out|timeout|statement timeout/i, 'timeout'],
  [
    /not running|no running container|container .* not found|is stopped/i,
    'notRunning',
  ],
]

const STATUS_TIMEOUT = 408
const STATUS_NOT_IMPLEMENTED = 501
const STATUS_CONFLICT = 409

// Maps a failed schema or rows call to a plain reason. The message is
// matched first because the API carries the database's own error text;
// the status code is the fallback.
export function classifyExplorerError(error: unknown): ExplorerErrorInfo {
  if (!(error instanceof ApiError)) {
    const raw = error instanceof Error ? error.message : String(error)
    return { reason: 'network', raw }
  }
  const raw = error.message
  for (const [re, reason] of PATTERNS) {
    if (re.test(raw)) return { reason, raw }
  }
  if (error.status === STATUS_TIMEOUT) return { reason: 'timeout', raw }
  if (error.status === STATUS_CONFLICT) return { reason: 'notRunning', raw }
  if (error.status === STATUS_NOT_IMPLEMENTED) {
    return { reason: 'unsupported', raw }
  }
  return { reason: 'helper', raw }
}
