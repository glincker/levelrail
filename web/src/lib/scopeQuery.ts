import { getEnvironmentScope } from './environmentScope'
import { getProjectScope } from './projectScope'

export interface ScopeParams {
  environment: string
  project: string
}

// The picker's current selection, the one source for query keys and request params.
export function currentScope(): ScopeParams {
  return { environment: getEnvironmentScope(), project: getProjectScope() }
}

export function isScoped(s: ScopeParams = currentScope()): boolean {
  return s.environment !== '' || s.project !== ''
}

// Sets the scope as query params, named as the apps list names them.
export function withScope(
  params: URLSearchParams,
  s: ScopeParams = currentScope(),
): URLSearchParams {
  if (s.environment) params.set('environment', s.environment)
  if (s.project) params.set('project', s.project)
  return params
}

// Key suffix: empty when unscoped so unscoped caches keep their old keys.
export function scopeKey(s: ScopeParams = currentScope()): readonly unknown[] {
  return isScoped(s) ? [s] : []
}

/** scopeUrl appends the scope to a path as a query string. */
export function scopeUrl(
  path: string,
  s: ScopeParams = currentScope(),
): string {
  const qs = withScope(new URLSearchParams(), s).toString()
  return qs ? `${path}?${qs}` : path
}
