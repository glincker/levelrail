import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { Tone } from '@/components/kit'
import { useIsRoot } from './useIsRoot'
import { httpsStatusQueryOptions } from '../queries/httpsStatus'
import { nodeListQueryOptions } from '../queries/nodes'
import { systemDoctorQueryOptions } from '../queries/systemDoctor'

export interface DashboardNotice {
  id: string
  tone: Tone
  title: string
  detail: string
  to: '/domains' | '/settings/system-status' | '/nodes'
  action: string
}

/** useDashboardNotices lists platform-level notices; disk and release notices live in next steps and the strip. */
export function useDashboardNotices(): DashboardNotice[] {
  const { t } = useTranslation('dashboard')
  const isRoot = useIsRoot()
  const https = useQuery({
    ...httpsStatusQueryOptions(),
    retry: false,
    enabled: isRoot && window.location.protocol === 'http:',
  })
  const doctor = useQuery({
    ...systemDoctorQueryOptions(),
    retry: false,
    staleTime: 60_000,
  })
  const nodes = useQuery({ ...nodeListQueryOptions(), retry: false })
  const offlineNodes = (nodes.data ?? []).filter((n) => n.status === 'offline')
  const plainHttp =
    https.data?.state === 'off' && Boolean(https.data.suggested_domain)
  const problems = (doctor.data?.checks ?? []).filter(
    (c) =>
      (c.status === 'fail' ||
        (c.status === 'warn' && c.class === 'incident')) &&
      !(plainHttp && c.code === 'dashboard_url_https'),
  )
  const out: DashboardNotice[] = []
  if (plainHttp) {
    out.push({
      id: 'https',
      tone: 'warning',
      title: t('banners.httpsTitle'),
      detail: t('banners.httpsDetail'),
      to: '/domains',
      action: t('banners.httpsAction'),
    })
  }
  if (problems.length > 0) {
    out.push({
      id: 'doctor',
      tone: problems.some((c) => c.status === 'fail') ? 'danger' : 'warning',
      title: t('banners.doctorTitle', { count: problems.length }),
      detail: t('banners.doctorDetail'),
      to: '/settings/system-status',
      action: t('banners.doctorAction'),
    })
  }
  if (offlineNodes.length > 0) {
    out.push({
      id: 'nodes-offline',
      tone: 'danger',
      title: t('banners.nodesTitle', { count: offlineNodes.length }),
      detail: t('banners.nodesDetail'),
      to: '/nodes',
      action: t('banners.nodesAction'),
    })
  }
  return out
}
