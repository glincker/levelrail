import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/ui/status-badge'
import { formatAge, formatSize } from '../../lib/format'
import { appListQueryOptions } from '../../queries/apps'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import { SiteUrlChip } from './SiteSuccess'

/** One horizontal row under the title: URL chip, status, last deploy, size. Unknown values are left out, never guessed. */
export function SiteHeaderMeta({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const apps = useQuery({ ...appListQueryOptions(), retry: false })
  const detail = usePagesSiteDetail(name).data
  const status = apps.data?.find((a) => a.name === name)?.status
  const lastAt =
    detail?.latest_deploy?.finished_at ??
    detail?.latest_deploy?.started_at ??
    detail?.updated_at
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm">
      <SiteUrlChip name={name} />
      {status ? (
        <StatusBadge variant={status.variant} label={status.label} />
      ) : null}
      {lastAt ? (
        <span className="text-muted-foreground">
          {t('detail.lastDeploy', { when: formatAge(lastAt) })}
        </span>
      ) : null}
      {detail ? (
        <span className="text-muted-foreground tabular-nums">
          {formatSize(detail.size_bytes)}
          {detail.file_count > 0
            ? ` \u00b7 ${t('detail.files', { count: detail.file_count })}`
            : ''}
        </span>
      ) : null}
    </div>
  )
}
