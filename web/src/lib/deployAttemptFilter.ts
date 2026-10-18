import type { DeployAttempt } from '../types/deployAttempt'

export interface DeployAttemptFilters {
  query: string
  branch: string
  author: string
  status: string
}

export const EMPTY_DEPLOY_FILTERS: DeployAttemptFilters = {
  query: '',
  branch: '',
  author: '',
  status: '',
}

export function hasActiveDeployFilters(f: DeployAttemptFilters): boolean {
  return Boolean(f.query || f.branch || f.author || f.status)
}

export function filterDeployAttempts(
  attempts: DeployAttempt[],
  f: DeployAttemptFilters,
): DeployAttempt[] {
  const q = f.query.trim().toLowerCase()
  return attempts.filter((a) => {
    if (f.branch && a.branch !== f.branch) return false
    if (f.author && a.author !== f.author) return false
    if (f.status && a.status !== f.status) return false
    if (!q) return true
    const hay =
      `${a.commit_message ?? ''} ${a.image} ${a.branch ?? ''} ${a.author ?? ''} ${a.commit_sha ?? ''}`.toLowerCase()
    return hay.includes(q)
  })
}

export function distinctDeployValues(
  attempts: DeployAttempt[],
  key: 'branch' | 'author' | 'status',
): string[] {
  const seen = new Set<string>()
  for (const a of attempts) {
    const v = a[key]
    if (v) seen.add(v)
  }
  return [...seen].sort()
}
