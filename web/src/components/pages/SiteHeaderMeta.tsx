import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/ui/status-badge'
import { formatAge, formatSize } from '../../lib/format'
import { appListQueryOptions } from '../../queries/apps'
import { deployAttemptsQueryOptions } from '../../queries/deployAttempts'
import { usePagesSites } from '../../queries/pagesSites'
import { SiteUrlChip } from './SiteSuccess'

/** One horizontal row under the title: URL chip, status, last deploy, size. Unknown values are left out, never guessed. */
export function SiteHeaderMeta({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const apps = useQuery({ ...appListQueryOptions(), retry: false })
  const attempts = useQuery({
    ...deployAttemptsQueryOptions(name),
    retry: false,
  })
  const sites = usePagesSites()
  const status = apps.data?.find((a) => a.name === name)?.status
  const site = sites.data?.find((s) => s.name === name)
  const last = attempts.data?.[0]
  const lastAt = site?.last_deploy_at ?? last?.finished_at ?? last?.started_at
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
      {site?.size_bytes !== undefined ? (
        <span className="text-muted-foreground tabular-nums">
          {formatSize(site.size_bytes)}
        </span>
      ) : null}
    </div>
  )
}
