import { useTranslation } from 'react-i18next'
import { buildReleaseRows } from '@/lib/appReleases'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import type { AppReleasesResponse } from '../../types/appRelease'
import { useAppReleases } from '../../queries/appReleases'
import { PinnedDigestBadge } from '../PinnedDigestBadge'

export function AppReleasesList({ data }: { data?: AppReleasesResponse }) {
  const { t } = useTranslation('common')
  if (!data || data.releases.length === 0) return null
  const rows = buildReleaseRows(data.releases, data.current_release_id)
  return (
    <section aria-labelledby="app-releases-title" className="space-y-2">
      <h2 id="app-releases-title" className="text-sm font-medium">
        {t('appReleases.title')}
      </h2>
      <ul className="divide-y divide-border rounded-lg border border-border">
        {rows.map(({ release, isCurrent }) => (
          <li
            key={release.id}
            className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm"
          >
            <PinnedDigestBadge digest={release.digest} />
            <span
              className="text-muted-foreground"
              title={formatAbsoluteTime(release.created_at)}
            >
              {formatRelativeTime(release.created_at)}
            </span>
            {release.source_reason && (
              <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                {release.source_reason}
              </span>
            )}
            {isCurrent && (
              <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                {t('appReleases.current')}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

export function AppReleases({ appName }: { appName: string }) {
  const { data } = useAppReleases(appName)
  return <AppReleasesList data={data} />
}
