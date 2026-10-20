import { useQuery } from '@tanstack/react-query'
import { isInsecureRemoteConnection } from '../../lib/connection'
import { certificatesQueryOptions } from '../../queries/certificates'
import { nodeListQueryOptions } from '../../queries/nodes'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { systemStatusQueryOptions } from '../../queries/systemStatus'
import { DASHBOARD_URL_ANCHOR } from '../DashboardUrlCard'
import { derivePlatformIssues, type PlatformIssue } from './platformIssues'

const REFRESH_MS = 30_000

// Same options as useAttentionItems so the shared query cache dedupes fetches.
export function usePlatformIssues(): PlatformIssue[] {
  const opts = { retry: false, refetchInterval: REFRESH_MS } as const
  const status = useQuery({ ...systemStatusQueryOptions(), ...opts })
  const nodes = useQuery({ ...nodeListQueryOptions(), ...opts })
  const certs = useQuery({ ...certificatesQueryOptions(), ...opts })
  const doctor = useQuery({ ...systemDoctorQueryOptions(), ...opts })
  return derivePlatformIssues({
    status: status.data,
    nodes: nodes.data,
    certs: certs.data,
    doctor: doctor.data,
    insecure: isInsecureRemoteConnection(window.location),
    dashboardUrlAnchor: DASHBOARD_URL_ANCHOR,
  })
}
