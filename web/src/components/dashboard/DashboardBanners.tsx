import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { useIsRoot } from '../../hooks/useIsRoot'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { updatesQueryOptions } from '../../queries/updates'
import { CleanUpDockerDialog } from '../CleanUpDockerDialog'
import { snoozeNow, useSnoozed } from '../shell/snoozeStore'
import type { PlatformIssue } from '../shell/platformIssues'
import { usePlatformIssues } from '../shell/usePlatformIssues'

const DISK_ISSUE_ID = 'disk'
const DISK_SNOOZE_ID = 'banner:disk'

function pickDiskIssue(
  issues: PlatformIssue[],
  isSnoozed: (id: string) => boolean,
): PlatformIssue | undefined {
  if (isSnoozed(DISK_SNOOZE_ID)) return undefined
  return issues.find((i) => i.id === DISK_ISSUE_ID && i.severity === 'critical')
}

/** DashboardBanners carries the notices the shell bar shows elsewhere: critical disk and a newer release. */
export function DashboardBanners() {
  const { t } = useTranslation('dashboard')
  const isRoot = useIsRoot()
  const issues = usePlatformIssues()
  const { isSnoozed } = useSnoozed()
  const updates = useQuery({
    ...updatesQueryOptions(),
    retry: false,
    staleTime: 60 * 60_000,
    enabled: isRoot,
  })
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
  const disk = pickDiskIssue(issues, isSnoozed)
  const update = updates.data?.update_available ? updates.data : undefined
  const plainHttp =
    https.data?.state === 'off' && Boolean(https.data.suggested_domain)
  const problems = (doctor.data?.checks ?? []).filter(
    (c) =>
      (c.status === 'fail' ||
        (c.status === 'warn' && c.class === 'incident')) &&
      !(plainHttp && c.code === 'dashboard_url_https'),
  )
  const failing = problems.some((c) => c.status === 'fail')
  if (!disk && !update && !plainHttp && problems.length === 0) return null

  return (
    <>
      {plainHttp ? (
        <Banner
          variant="warning"
          title={t('banners.httpsTitle')}
          dismissId="dashboard-enable-https"
          action={
            <Button size="sm" variant="outline" render={<Link to="/domains" />}>
              {t('banners.httpsAction')}
            </Button>
          }
        >
          {t('banners.httpsDetail')}
        </Banner>
      ) : null}
      {disk ? (
        <Banner
          variant="critical"
          title={disk.title}
          action={<CleanUpDockerDialog />}
          onDismiss={() => {
            snoozeNow(DISK_SNOOZE_ID, '1d')
          }}
        >
          {t('banners.diskDetail', { detail: disk.detail })}
        </Banner>
      ) : null}
      {problems.length > 0 ? (
        <Banner
          variant={failing ? 'critical' : 'warning'}
          title={t('banners.doctorTitle', { count: problems.length })}
          action={
            <Button
              size="sm"
              variant="outline"
              render={<Link to="/settings/system-status" />}
            >
              {t('banners.doctorAction')}
            </Button>
          }
        >
          {t('banners.doctorDetail')}
        </Banner>
      ) : null}
      {update ? (
        <Banner
          variant="info"
          title={t('banners.updateTitle', {
            version: update.latest_version ?? '',
          })}
          dismissId={`dashboard-update:${update.latest_version ?? 'latest'}`}
          action={
            <Button
              size="sm"
              variant="outline"
              render={<Link to="/settings/updates" />}
            >
              {t('banners.updateAction')}
            </Button>
          }
        >
          {t('banners.updateDetail', { current: update.current_version })}
        </Banner>
      ) : null}
    </>
  )
}
