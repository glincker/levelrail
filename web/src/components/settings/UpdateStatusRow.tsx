import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowUpGlassIcon,
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  InfoGlassIcon,
} from '@thesvg/ui-react'
import { preflightQueryOptions, type UpdateStatus } from '../../queries/updates'

export function UpdateStatusRow({ status }: { status: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const edge = status.release_kind === 'edge_image'
  const hostBinaryEdge = edge && preflight.data?.edge?.installable === false

  if (hostBinaryEdge) {
    return (
      <div className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
        <InfoGlassIcon className="size-4.5 text-tone-info" />
        {t('version.edgeHostBinary')}
      </div>
    )
  }

  if (status.latest_version === null) {
    return <p className="text-sm text-muted-foreground">{t('version.none')}</p>
  }

  if (!status.update_available) {
    return (
      <div className="inline-flex items-center gap-1.5 text-sm text-green-700 dark:text-green-400">
        <CircleCheckGlassIcon className="size-4.5 text-tone-success" />
        {edge ? t('version.upToDateEdge') : t('version.upToDate')}
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="inline-flex items-center gap-1.5 text-sm text-foreground">
        <ArrowUpGlassIcon className="size-4.5" />
        {t(edge ? 'version.latestEdge' : 'version.latest', {
          version: status.latest_version,
        })}
      </div>
      {!edge && status.release_url && (
        <a
          href={status.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
        >
          {t('version.viewRelease')}
          <ExternalLinkGlassIcon className="size-4" />
        </a>
      )}
    </div>
  )
}
