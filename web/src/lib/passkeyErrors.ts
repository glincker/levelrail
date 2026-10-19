export type PasskeyErrorKind =
  | 'cancelled'
  | 'timeout'
  | 'alreadyRegistered'
  | 'unsupported'
  | 'insecureOrigin'
  | 'rejected'
  | 'rateLimited'
  | 'unavailable'
  | 'lastMethod'
  | 'network'
  | 'unknown'

function errorName(error: unknown): string {
  return typeof error === 'object' && error !== null && 'name' in error
    ? String(error.name)
    : ''
}

function errorStatus(error: unknown): number | null {
  if (typeof error !== 'object' || error === null || !('status' in error)) {
    return null
  }
  const status = error.status
  return typeof status === 'number' ? status : null
}

// Browsers report a dismissed prompt and a timed out one as the same NotAllowedError, so the message is the only hint.
export function classifyPasskeyError(error: unknown): PasskeyErrorKind {
  const name = errorName(error)
  const message = error instanceof Error ? error.message.toLowerCase() : ''
  switch (name) {
    case 'NotAllowedError':
      return message.includes('timed out') || message.includes('timeout')
        ? 'timeout'
        : 'cancelled'
    case 'AbortError':
      return 'cancelled'
    case 'InvalidStateError':
      return 'alreadyRegistered'
    case 'SecurityError':
      return 'insecureOrigin'
    case 'NotSupportedError':
    case 'PasskeyUnsupportedError':
      return 'unsupported'
    case 'TimeoutError':
      return 'timeout'
    default:
      break
  }
  const status = errorStatus(error)
  if (status === 409) return 'lastMethod'
  if (status === 429) return 'rateLimited'
  if (status === 501) return 'unavailable'
  if (status !== null && status >= 400 && status < 500) return 'rejected'
  if (error instanceof TypeError) return 'network'
  return 'unknown'
}

// Whether trying the same ceremony again can plausibly help.
export function isRetryablePasskeyError(kind: PasskeyErrorKind): boolean {
  return (
    kind === 'cancelled' ||
    kind === 'timeout' ||
    kind === 'network' ||
    kind === 'rejected' ||
    kind === 'unknown'
  )
}
