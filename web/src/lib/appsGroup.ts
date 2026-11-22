import type { AppListEntry } from '../types/appDetail'

export type GroupBy = 'none' | 'node' | 'project' | 'environment'
export const GROUP_BY_OPTIONS: GroupBy[] = [
  'none',
  'node',
  'project',
  'environment',
]

export interface AppGroup {
  key: string
  // Empty when the apps have no value for this field; the UI supplies the "none" text.
  label: string
  apps: AppListEntry[]
}

const KEY = 'apps.groupBy.v1'

export function loadGroupBy(): GroupBy {
  try {
    const raw = window.localStorage.getItem(KEY)
    return (GROUP_BY_OPTIONS as string[]).includes(raw ?? '')
      ? (raw as GroupBy)
      : 'none'
  } catch {
    return 'none'
  }
}

export function storeGroupBy(by: GroupBy): void {
  try {
    window.localStorage.setItem(KEY, by)
  } catch {
    // Grouping is a convenience only.
  }
}

// Client side over the rows already loaded. Named groups sort A to Z, the
// ungrouped bucket (empty label) goes last. Row order inside a group is kept.
export function groupApps(
  apps: readonly AppListEntry[],
  by: Exclude<GroupBy, 'none'>,
  projectNames: ReadonlyMap<string, string> = new Map(),
): AppGroup[] {
  const groups = new Map<string, AppGroup>()
  for (const app of apps) {
    const label =
      by === 'node'
        ? (app.node_name ?? app.node_id ?? '')
        : by === 'project'
          ? (projectNames.get(app.project_id ?? '') ?? app.project_id ?? '')
          : (app.environment_name ?? '')
    const group = groups.get(label) ?? { key: label, label, apps: [] }
    group.apps.push(app)
    groups.set(label, group)
  }
  return [...groups.values()].sort((a, b) =>
    a.label === '' ? 1 : b.label === '' ? -1 : a.label.localeCompare(b.label),
  )
}
