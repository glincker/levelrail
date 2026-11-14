import { ApiError, routeErrorMessage } from './apiError'
import { browserInfo, redactText, routeTemplate } from './issueReport'

const STACK_LINES = 5
const STACK_MAX_CHARS = 600

// Plain text for a bug report: no tokens, headers or bodies, and a short redacted stack.
export function errorReport(
  error: unknown,
  header: string,
  version?: string,
): string {
  const api = error instanceof ApiError ? error : undefined
  const b = browserInfo(navigator)
  const stack =
    error instanceof Error && error.stack
      ? redactText(
          error.stack.split('\n').slice(0, STACK_LINES).join('\n'),
        ).slice(0, STACK_MAX_CHARS)
      : null
  return [
    header,
    `message: ${redactText(routeErrorMessage(error))}`,
    api ? `status: ${api.status}` : null,
    api?.code ? `code: ${api.code}` : null,
    api?.requestId ? `request_id: ${api.requestId}` : null,
    `route: ${routeTemplate(window.location.pathname)}`,
    version ? `version: ${version}` : null,
    `browser: ${b.browser} on ${b.os}`,
    `time: ${new Date().toISOString()}`,
    stack ? `stack:\n${stack}` : null,
  ]
    .filter((l): l is string => l !== null)
    .join('\n')
}
