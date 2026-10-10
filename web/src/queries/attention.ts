import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useNow } from '../hooks/useNow'
import { buildAttentionItems } from '../lib/attention'
import { buildWaitingItems, rankAttentionItems } from '../lib/attentionWaiting'
import { deviceAuthRequestsQueryOptions } from './deviceAuth'
import { deployApprovalListQueryOptions } from './deployApprovals'
import { appListQueryOptions } from './apps'
import { certificatesQueryOptions } from './certificates'
import { nodeListQueryOptions } from './nodes'
import { assessDiskPressure } from '../lib/diskPressure'
import { failedDeploysQueryOptions } from './failedDeploys'
import { systemDoctorQueryOptions } from './systemDoctor'
import { systemStatusQueryOptions } from './systemStatus'
import { updatesQueryOptions } from './updates'

const REFRESH_MS = 30_000
const DEVICE_REFRESH_MS = 5_000
const NOW_TICK_MS = 15_000

// Each source is optional: a failing or forbidden endpoint drops its own
// items instead of blocking the whole view.
export function useAttentionItems() {
  const opts = { retry: false, refetchInterval: REFRESH_MS } as const
  const apps = useQuery({ ...appListQueryOptions(), ...opts })
  const nodes = useQuery({ ...nodeListQueryOptions(), ...opts })
  const certs = useQuery({ ...certificatesQueryOptions(), ...opts })
  const doctor = useQuery({ ...systemDoctorQueryOptions(), ...opts })
  const failed = useQuery({ ...failedDeploysQueryOptions(), ...opts })
  const status = useQuery({ ...systemStatusQueryOptions(), ...opts })
  const updates = useQuery({ ...updatesQueryOptions(), ...opts })
  const devices = useQuery({
    ...deviceAuthRequestsQueryOptions(),
    retry: false,
    refetchInterval: DEVICE_REFRESH_MS,
  })
  const approvals = useQuery({
    ...deployApprovalListQueryOptions('pending'),
    ...opts,
  })
  const { t } = useTranslation('attention', { useSuspense: false })
  const now = useNow(NOW_TICK_MS)

  const base = buildAttentionItems({
    apps: apps.data,
    nodes: nodes.data,
    certs: certs.data,
    doctor: doctor.data,
    failedDeploys: failed.data,
    disk: status.data ? assessDiskPressure(status.data) : undefined,
    updates: updates.data,
  })
  const waiting = buildWaitingItems({
    deviceLogins: devices.data,
    approvals: approvals.data,
    certs: certs.data,
    nodes: nodes.data,
    now,
    t,
  })
  const items = rankAttentionItems([...base, ...waiting])

  return {
    items,
    isLoading:
      apps.isLoading || nodes.isLoading || certs.isLoading || doctor.isLoading,
  }
}
