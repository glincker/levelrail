import { useQueries } from '@tanstack/react-query'
import type { SetupChecklistInput } from '../lib/setupChecklist'
import { fetchAlertRules } from './alerts'
import { fetchApps } from './apps'
import { fetchBackupTargets } from './backupTargets'
import { fetchCertificates } from './certificates'
import { fetchControlPlaneBackups } from './controlPlaneBackups'
import { fetchDashboardUrl } from './dashboardUrl'
import { fetchDomains } from './domains'
import { fetchGitProviders } from './gitProviders'
import { fetchNotificationChannels } from './notificationChannels'
import { fetchTwoFactorStatus } from './twoFactor'

const STALE_TIME = 60_000
const KEY = 'setup-checklist'

// Any query error (403, 404, 501, network) means "unavailable", never a
// thrown error: the card must not break the dashboard.
function slice<T>(q: {
  data: T | undefined
  isPending: boolean
}): T | null | undefined {
  if (q.isPending) return undefined
  return q.data === undefined ? null : q.data
}

// Platform-wide alert rules hang off any app, so the first app carries them.
export function useSetupChecklistInput(
  enabled: boolean,
  firstAppName: string | undefined,
): SetupChecklistInput {
  const opts = { staleTime: STALE_TIME, retry: false, enabled }
  const [
    sources,
    apps,
    domains,
    certs,
    targets,
    cpBackups,
    channels,
    rules,
    tfa,
    url,
  ] = useQueries({
    queries: [
      { queryKey: [KEY, 'sources'], queryFn: fetchGitProviders, ...opts },
      { queryKey: [KEY, 'apps'], queryFn: fetchApps, ...opts },
      { queryKey: [KEY, 'domains'], queryFn: fetchDomains, ...opts },
      { queryKey: [KEY, 'certs'], queryFn: fetchCertificates, ...opts },
      { queryKey: [KEY, 'targets'], queryFn: fetchBackupTargets, ...opts },
      {
        queryKey: [KEY, 'cp-backups'],
        queryFn: fetchControlPlaneBackups,
        ...opts,
      },
      {
        queryKey: [KEY, 'channels'],
        queryFn: fetchNotificationChannels,
        ...opts,
      },
      {
        queryKey: [KEY, 'rules', firstAppName ?? ''],
        queryFn: () => fetchAlertRules(firstAppName ?? ''),
        ...opts,
        enabled: enabled && firstAppName !== undefined,
      },
      { queryKey: [KEY, '2fa'], queryFn: fetchTwoFactorStatus, ...opts },
      { queryKey: [KEY, 'url'], queryFn: fetchDashboardUrl, ...opts },
    ],
  })
  return {
    gitProviders: slice(sources),
    apps: slice(apps),
    domains: slice(domains),
    certificates: slice(certs),
    backupTargets: slice(targets),
    controlPlaneBackups: slice(cpBackups),
    channels: slice(channels),
    alertRules: firstAppName === undefined ? [] : slice(rules),
    twoFactor: slice(tfa),
    dashboardUrl: slice(url),
  }
}
