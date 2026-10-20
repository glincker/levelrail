import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowCircleUpIcon,
  ArrowSquareOutIcon,
  CheckCircleIcon,
  InfoIcon,
} from '@phosphor-icons/react/dist/ssr'
import { preflightQueryOptions, type UpdateStatus } from '../../queries/updates'

export function UpdateStatusRow({ status }: { status: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const edge = status.release_kind === 'edge_image'
  const hostBinaryEdge = edge && preflight.data?.edge?.installable === false

  if (hostBinaryEdge) {
    return (
      <div className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
        <InfoIcon className="size-4" />
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
        <CheckCircleIcon className="size-4" />
        {edge ? t('version.upToDateEdge') : t('version.upToDate')}
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="inline-flex items-center gap-1.5 text-sm text-foreground">
        <ArrowCircleUpIcon className="size-4" />
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
          <ArrowSquareOutIcon className="size-3.5" />
        </a>
      )}
    </div>
  )
}
