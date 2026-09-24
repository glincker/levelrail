import type { AppListEntry } from '../types/appDetail'
import type { AlertRule } from '../types/alerts'
import type { BackupTarget } from '../types/backupTarget'
import type { GitProviderStatus } from '../types/gitProviders'
import type { NotificationChannel } from '../types/notificationChannel'
import type { TwoFactorStatus } from '../types/twoFactor'
import type { CertificateStatus } from '../queries/certificates'
import type { ControlPlaneBackup } from '../queries/controlPlaneBackups'
import type { DashboardUrlSetting } from '../queries/dashboardUrl'
import type { Domain } from '../queries/domains'

export type SetupItemId =
  | 'source'
  | 'app'
  | 'domain'
  | 'backup'
  | 'alerts'
  | 'twoFactor'
  | 'dashboardUrl'

export type SetupItemState = 'done' | 'todo' | 'unavailable'

// Each field is undefined while loading and null when the endpoint is
// unavailable to this operator (403, 404, 501, network error).
export interface SetupChecklistInput {
  gitProviders?: GitProviderStatus[] | null
  apps?: AppListEntry[] | null
  domains?: Domain[] | null
  certificates?: CertificateStatus[] | null
  backupTargets?: BackupTarget[] | null
  controlPlaneBackups?: ControlPlaneBackup[] | null
  channels?: NotificationChannel[] | null
  alertRules?: AlertRule[] | null
  twoFactor?: TwoFactorStatus | null
  dashboardUrl?: DashboardUrlSetting | null
}

export interface SetupItem {
  id: SetupItemId
  state: SetupItemState
}

export interface SetupChecklist {
  items: SetupItem[]
  loading: boolean
  done: number
  total: number
  complete: boolean
}

export const SETUP_ITEM_ORDER: SetupItemId[] = [
  'source',
  'app',
  'domain',
  'backup',
  'alerts',
  'twoFactor',
  'dashboardUrl',
]

function bool(done: boolean): SetupItemState {
  return done ? 'done' : 'todo'
}

function stateFor(
  id: SetupItemId,
  i: SetupChecklistInput,
): SetupItemState | undefined {
  switch (id) {
    case 'source':
      if (i.gitProviders === undefined) return undefined
      if (i.gitProviders === null) return 'unavailable'
      return bool(i.gitProviders.some((p) => p.connected))
    case 'app':
      if (i.apps === undefined) return undefined
      if (i.apps === null) return 'unavailable'
      return bool(i.apps.length > 0)
    case 'domain': {
      if (i.domains === undefined || i.certificates === undefined) {
        return undefined
      }
      if (i.domains === null) return 'unavailable'
      if (i.domains.length === 0) return 'todo'
      // Without a readable cert list, a configured domain is the best signal.
      if (i.certificates === null) return 'done'
      const names = new Set(i.domains.map((d) => d.domain))
      return bool(
        i.certificates.some(
          (c) =>
            c.status === 'healthy' &&
            (names.has(c.domain) || (c.sans ?? []).some((s) => names.has(s))),
        ),
      )
    }
    case 'backup':
      if (
        i.backupTargets === undefined ||
        i.controlPlaneBackups === undefined
      ) {
        return undefined
      }
      if (i.backupTargets === null) return 'unavailable'
      if (i.backupTargets.length === 0) return 'todo'
      if (i.controlPlaneBackups === null) return 'done'
      return bool(i.controlPlaneBackups.length > 0)
    case 'alerts':
      if (i.channels === undefined || i.alertRules === undefined) {
        return undefined
      }
      if (i.channels === null || i.alertRules === null) return 'unavailable'
      return bool(i.channels.length > 0 && i.alertRules.length > 0)
    case 'twoFactor':
      if (i.twoFactor === undefined) return undefined
      if (i.twoFactor === null) return 'unavailable'
      return bool(i.twoFactor.enabled)
    case 'dashboardUrl':
      if (i.dashboardUrl === undefined) return undefined
      if (i.dashboardUrl === null) return 'unavailable'
      return bool(i.dashboardUrl.dashboard_url.trim() !== '')
  }
}

export function computeSetupChecklist(
  input: SetupChecklistInput,
): SetupChecklist {
  let loading = false
  const items: SetupItem[] = []
  for (const id of SETUP_ITEM_ORDER) {
    const state = stateFor(id, input)
    if (state === undefined) {
      loading = true
      continue
    }
    items.push({ id, state })
  }
  const counted = items.filter((it) => it.state !== 'unavailable')
  const done = counted.filter((it) => it.state === 'done').length
  return {
    items,
    loading,
    done,
    total: counted.length,
    complete: !loading && counted.length > 0 && done === counted.length,
  }
}
