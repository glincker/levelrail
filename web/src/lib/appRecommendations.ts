export const MAX_RECOMMENDATIONS = 3

export type RecommendationId =
  | 'failed_deploy'
  | 'add_domain'
  | 'enable_backups'
  | 'add_health_check'
  | 'enable_previews'

export type Recommendation =
  | {
      id: RecommendationId
      to:
        | '/apps/$name/domains'
        | '/apps/$name/health'
        | '/apps/$name/previews'
        | '/databases/$name'
      params: { name: string }
    }
  | {
      id: RecommendationId
      to: '/apps/$name/deploys/$deployId/logs'
      params: { name: string; deployId: string }
    }

export interface RecommendationInput {
  appName: string
  hasDomain: boolean
  hasHealthCheck: boolean
  latestDeploy?: { id: string; status: string }
  /** Attached databases and whether each has a backup schedule. */
  databases?: readonly { name: string; hasSchedule: boolean }[]
  /** A git source (the pull request flow) is connected. */
  hasGitSource?: boolean
  /** Undefined while unknown, so nothing is suggested from a guess. */
  previewsEnabled?: boolean
  dismissed: ReadonlySet<string>
}

// Order is priority: a broken deploy beats every setup nudge.
export function computeRecommendations(
  input: RecommendationInput,
): Recommendation[] {
  const out: Recommendation[] = []
  const app = { name: input.appName }
  const latest = input.latestDeploy
  if (latest?.status === 'failed') {
    out.push({
      id: 'failed_deploy',
      to: '/apps/$name/deploys/$deployId/logs',
      params: { ...app, deployId: latest.id },
    })
  }
  if (!input.hasDomain) {
    out.push({ id: 'add_domain', to: '/apps/$name/domains', params: app })
  }
  const unprotected = input.databases?.find((d) => !d.hasSchedule)
  if (unprotected) {
    out.push({
      id: 'enable_backups',
      to: '/databases/$name',
      params: { name: unprotected.name },
    })
  }
  if (!input.hasHealthCheck) {
    out.push({ id: 'add_health_check', to: '/apps/$name/health', params: app })
  }
  if (input.hasGitSource === true && input.previewsEnabled === false) {
    out.push({
      id: 'enable_previews',
      to: '/apps/$name/previews',
      params: app,
    })
  }
  return out
    .filter((r) => !input.dismissed.has(r.id))
    .slice(0, MAX_RECOMMENDATIONS)
}

const storageKey = (app: string) => `app.recommended.dismissed.${app}`

export function readDismissed(app: string): Set<string> {
  try {
    const raw = window.localStorage.getItem(storageKey(app))
    const parsed: unknown = raw ? JSON.parse(raw) : []
    return new Set(
      Array.isArray(parsed)
        ? parsed.filter((v): v is string => typeof v === 'string')
        : [],
    )
  } catch {
    return new Set()
  }
}

export function writeDismissed(app: string, ids: ReadonlySet<string>): void {
  try {
    window.localStorage.setItem(storageKey(app), JSON.stringify([...ids]))
  } catch {
    // storage unavailable: the dismissal lasts for this page view only
  }
}
