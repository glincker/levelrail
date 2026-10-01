import { useQuery } from '@tanstack/react-query'
import { buildAttentionItems } from '../lib/attention'
import { appListQueryOptions } from './apps'
import { certificatesQueryOptions } from './certificates'
import { nodeListQueryOptions } from './nodes'
import { assessDiskPressure } from '../lib/diskPressure'
import { failedDeploysQueryOptions } from './failedDeploys'
import { systemDoctorQueryOptions } from './systemDoctor'
import { systemStatusQueryOptions } from './systemStatus'
import { updatesQueryOptions } from './updates'

const REFRESH_MS = 30_000

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

  return {
    items: buildAttentionItems({
      apps: apps.data,
      nodes: nodes.data,
      certs: certs.data,
      doctor: doctor.data,
      failedDeploys: failed.data,
      disk: status.data ? assessDiskPressure(status.data) : undefined,
      updates: updates.data,
    }),
    isLoading:
      apps.isLoading || nodes.isLoading || certs.isLoading || doctor.isLoading,
  }
}
