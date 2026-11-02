// Table from stable API error codes, HTTP statuses and message patterns to a
// friendly kind. Strings live in common.json under apiError.friendly.<kind>.

export const ERROR_KINDS = [
  'generic',
  'docker',
  'unauthorized',
  'forbidden',
  'missingAbility',
  'conflict',
  'validation',
  'rateLimited',
  'diskFull',
  'unreachable',
  'network',
  'timeout',
  'notFound',
  'tooLarge',
  'serverError',
  'needsConfirmation',
  'domainTaken',
  'emailTaken',
  'stepUp',
  'portConflict',
  'emailNotConfigured',
] as const

export type ErrorKind = (typeof ERROR_KINDS)[number]

export const CODE_KIND: Record<string, ErrorKind> = {
  docker_unavailable: 'docker',
  unauthorized: 'unauthorized',
  forbidden: 'forbidden',
  conflict: 'conflict',
  validation: 'validation',
  validation_failed: 'validation',
  invalid: 'validation',
  invalid_token: 'validation',
  confirmation_mismatch: 'validation',
  rate_limited: 'rateLimited',
  too_many_requests: 'rateLimited',
  step_up_locked: 'rateLimited',
  export_cooldown: 'rateLimited',
  email_change_limited: 'rateLimited',
  daily_cap_reached: 'rateLimited',
  disk_full: 'diskFull',
  insufficient_storage: 'diskFull',
  not_found: 'notFound',
  payload_too_large: 'tooLarge',
  unavailable: 'unreachable',
  internal_error: 'serverError',
  timeout: 'timeout',
  request_timeout: 'timeout',
  requires_confirmation: 'needsConfirmation',
  domain_taken: 'domainTaken',
  email_taken: 'emailTaken',
  step_up_code_required: 'stepUp',
  step_up_invalid: 'stepUp',
  step_up_no_passkey: 'stepUp',
  step_up_session_only: 'stepUp',
  published_port_conflict: 'portConflict',
  email_not_configured: 'emailNotConfigured',
}

export const STATUS_KIND: Record<number, ErrorKind> = {
  400: 'validation',
  401: 'unauthorized',
  403: 'forbidden',
  404: 'notFound',
  408: 'timeout',
  409: 'conflict',
  413: 'tooLarge',
  422: 'validation',
  429: 'rateLimited',
  500: 'serverError',
  502: 'unreachable',
  503: 'unreachable',
  504: 'unreachable',
  507: 'diskFull',
}

// Server text that names the exact field or thing wins over the fixed sentence.
export const KEEP_SERVER_TEXT: ReadonlySet<ErrorKind> = new Set([
  'validation',
  'conflict',
  'notFound',
  'serverError',
])

// Kinds a retry can fix, so the toast offers one.
export const RETRYABLE: ReadonlySet<ErrorKind> = new Set([
  'unreachable',
  'network',
  'timeout',
  'serverError',
])

const GENERIC_MESSAGES = new Set(['internal error', ''])
const MAX_SAFE_LENGTH = 200
const UNSAFE_TEXT = /[<>\n\r]|goroutine|panic:|\.go:\d+|\/internal\/|sql:/i

// Plain, short, single-line sentences are safe to show; anything that looks
// like a stack, path or markup is replaced by the generic text.
export function isSafeServerText(raw: string): boolean {
  const text = raw.trim()
  return (
    text !== '' &&
    !GENERIC_MESSAGES.has(text.toLowerCase()) &&
    text.length <= MAX_SAFE_LENGTH &&
    !UNSAFE_TEXT.test(text)
  )
}

// Pulls the missing ability out of messages like "token lacks the required
// ability (needs write:apps)" or "requires the root ability".
const ABILITY_PATTERNS: RegExp[] = [
  /\(needs ([a-z][a-z0-9:_-]*)\)/i,
  /needs the ([a-z][a-z0-9:_-]*) ability/i,
  /requires the ([a-z][a-z0-9:_-]*) ability/i,
]

export function abilityFromMessage(message: string): string | undefined {
  for (const re of ABILITY_PATTERNS) {
    const hit = re.exec(message)
    if (hit?.[1]) return hit[1]
  }
  return undefined
}

export function isTimeoutError(err: unknown): boolean {
  return (
    err instanceof DOMException &&
    (err.name === 'TimeoutError' || err.name === 'AbortError')
  )
}
