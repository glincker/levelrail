import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  accountExportDownloadUrl,
  accountExportQueryOptions,
  useStartAccountExport,
} from '../../queries/accountSelf'
import { formatSize } from '../../lib/format'

export function DataExportCard() {
  const { t } = useTranslation('account')
  const [jobId, setJobId] = useState<string | null>(null)
  const start = useStartAccountExport()
  const job = useQuery(accountExportQueryOptions(jobId))
  const status = job.data?.status

  const begin = () =>
    start.mutate(undefined, { onSuccess: (created) => setJobId(created.id) })

  const failed = start.isError || status === 'failed' || job.isError
  const busy = start.isPending || status === 'pending'

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('export.title')}</CardTitle>
        <CardDescription>{t('export.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t('export.noSecrets')}</p>
        {failed ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>
                {start.isError ? t('export.startFailed') : t('export.failed')}
              </span>
              <Button type="button" size="sm" variant="outline" onClick={begin}>
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {status === 'ready' && jobId !== null ? (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm" role="status">
              {t('export.ready', {
                size: formatSize(job.data?.size_bytes ?? 0),
              })}
            </p>
            <a
              className={buttonVariants({ size: 'sm' })}
              href={accountExportDownloadUrl(jobId)}
              download
            >
              <DownloadGlassIcon size="1.4em" aria-hidden="true" />
              {t('export.download')}
            </a>
            <Button type="button" size="sm" variant="outline" onClick={begin}>
              {t('export.again')}
            </Button>
          </div>
        ) : (
          <Button
            type="button"
            size="sm"
            onClick={begin}
            disabled={busy}
            aria-busy={busy}
          >
            {busy ? t('export.preparing') : t('export.start')}
          </Button>
        )}
        {status === 'ready' ? (
          <p className="text-xs text-muted-foreground">{t('export.expires')}</p>
        ) : null}
      </CardContent>
    </Card>
  )
}
