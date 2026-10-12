import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { useIsRoot } from '../../hooks/useIsRoot'
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
  const disk = pickDiskIssue(issues, isSnoozed)
  const update = updates.data?.update_available ? updates.data : undefined
  if (!disk && !update) return null

  return (
    <>
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
