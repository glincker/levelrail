export type KeepItemId = 'domain' | 'backup' | 'alert' | 'placement'

export type KeepItemState = 'done' | 'todo' | 'loading' | 'none'

export interface KeepItem {
  id: KeepItemId
  state: KeepItemState
  /** Interpolation values for the item's status line. */
  values: Record<string, string | number>
  /** Which status line variant to show when done. */
  variant?: 'projectOnly'
}

export interface KeepChecklistInput {
  domains?: string[]
  /** One entry per volume: true when it has a backup schedule, undefined
   *  while still loading. */
  volumeSchedules: (boolean | undefined)[]
  /** Number of alert rules, undefined while loading. */
  alertCount?: number
  projectName?: string
  environmentName?: string
}

// What a permanent app should have, from data the app pages already load.
export function computeKeepChecklist(input: KeepChecklistInput): KeepItem[] {
  const domain = input.domains?.[0]
  const scheduled = input.volumeSchedules.filter(Boolean).length
  const backupLoading = input.volumeSchedules.some((s) => s === undefined)
  let backupState: KeepItemState = scheduled > 0 ? 'done' : 'todo'
  if (input.volumeSchedules.length === 0) backupState = 'none'
  else if (backupLoading && scheduled === 0) backupState = 'loading'

  let alertState: KeepItemState = 'loading'
  if (input.alertCount !== undefined) {
    alertState = input.alertCount > 0 ? 'done' : 'todo'
  }

  return [
    {
      id: 'domain',
      state: domain ? 'done' : 'todo',
      values: { domain: domain ?? '' },
    },
    {
      id: 'backup',
      state: backupState,
      values: { count: scheduled, total: input.volumeSchedules.length },
    },
    {
      id: 'alert',
      state: alertState,
      values: { count: input.alertCount ?? 0 },
    },
    {
      id: 'placement',
      state: input.projectName ? 'done' : 'todo',
      values: {
        project: input.projectName ?? '',
        environment: input.environmentName ?? '',
      },
      variant:
        input.projectName && !input.environmentName ? 'projectOnly' : undefined,
    },
  ]
}
