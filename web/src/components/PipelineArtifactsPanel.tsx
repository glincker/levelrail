import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon, PackageGlassIcon } from '@thesvg/ui-react'
import { buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSize } from '../lib/format'
import {
  artifactDownloadUrl,
  usePipelineArtifacts,
} from '../queries/pipelineExtras'

export function PipelineArtifactsPanel({
  app,
  runId,
}: {
  app: string
  runId: string
}) {
  const { t } = useTranslation('pipelines')
  const { data, isLoading, error } = usePipelineArtifacts(app, runId)
  const list = data ?? []
  return (
    <section aria-labelledby="run-artifacts" className="space-y-2">
      <h3
        id="run-artifacts"
        className="flex items-center gap-1.5 text-sm font-semibold text-foreground"
      >
        <PackageGlassIcon
          className="size-5 text-muted-foreground"
          aria-hidden="true"
        />
        {t('artifacts.title')}
      </h3>
      {isLoading ? (
        <Skeleton className="h-10 w-full" />
      ) : error ? (
        <p className="text-sm text-destructive">
          {t('artifacts.loadFailed')}: {error.message}
        </p>
      ) : list.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('artifacts.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {list.map((a) => (
            <li
              key={a.id}
              className="flex flex-wrap items-center gap-x-4 gap-y-1 p-3 text-sm"
            >
              <span className="min-w-24 flex-1 break-all font-medium text-foreground">
                {a.job}
              </span>
              <span className="tabular-nums text-muted-foreground">
                {formatSize(a.size_bytes)}
              </span>
              <code
                className="max-w-40 truncate text-xs text-muted-foreground"
                title={a.sha256}
              >
                {t('artifacts.sha')} {a.sha256.slice(0, 12)}
              </code>
              <span className="text-xs text-muted-foreground">
                {t('artifacts.retention', {
                  date: new Date(a.expires_at).toLocaleDateString(),
                })}
              </span>
              <a
                href={artifactDownloadUrl(app, runId, a.id)}
                download
                aria-label={t('artifacts.downloadLabel', { job: a.job })}
                className={buttonVariants({ variant: 'outline', size: 'sm' })}
              >
                <DownloadGlassIcon size="1.4em" aria-hidden="true" />
                {t('artifacts.download')}
              </a>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
