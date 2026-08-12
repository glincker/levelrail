// Shared types for the /apps resource. Deliberately a summary shape only:
// per frontend-plan.md section 3, the app list query fetches summary rows
// (id, name, status, primary domain, last deploy time/status), never the
// full app.yaml spec, env vars, or deploy history. Those live in their own
// types once the detail routes that need them are built.

export type AppStatus =
  'running' | 'stopped' | 'deploying' | 'crashloop' | 'unknown'

export interface AppSummary {
  id: string
  name: string
  status: AppStatus
  primaryDomain: string | null
  lastDeployAt: string | null
  lastDeployStatus: AppStatus | null
}

export interface AppPage {
  items: AppSummary[]
  nextCursor: string | null
}

export interface AppListFilters {
  search?: string
  status?: AppStatus
}
