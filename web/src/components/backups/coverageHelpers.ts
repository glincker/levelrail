import type { CoverageRow, CoverageState } from '../../types/backupCoverage'

export function coverageBadgeVariant(
  state: CoverageState,
): 'success' | 'info' | 'warning' | 'destructive' {
  switch (state) {
    case 'verified':
      return 'success'
    case 'unverified':
      return 'info'
    case 'never_ran':
      return 'warning'
    default:
      return 'destructive'
  }
}

export function rowKey(row: CoverageRow): string {
  return `${row.kind}:${row.app_name ?? ''}:${row.name}`
}

export function rowLabel(row: CoverageRow): string {
  return row.app_name ? `${row.app_name}/${row.name}` : row.name
}

/** coverageHeadline picks the one sentence the summary leads with. */
export type Headline =
  'empty' | 'allVerified' | 'someUnprotected' | 'failing' | 'unverified'

export function coverageHeadline(s: {
  total: number
  verified: number
  failing: number
  unprotected: number
}): Headline {
  if (s.total === 0) return 'empty'
  if (s.failing > 0) return 'failing'
  if (s.unprotected > 0) return 'someUnprotected'
  if (s.verified === s.total) return 'allVerified'
  return 'unverified'
}

/** Rows above this count render windowed. */
export const VIRTUALIZE_AFTER = 50
