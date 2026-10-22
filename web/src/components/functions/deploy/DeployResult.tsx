import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  SpinnerGapIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { deployAttemptsQueryOptions } from '../../../queries/deployAttempts'
import { functionsQueryOptions } from '../../../queries/functions'
import { CopyButton } from '../FunctionTemplates'

const URL_POLL_INTERVAL_MS = 3_000

export function DeployResult({
  name,
  fallbackUrl,
  onClose,
}: Readonly<{
  name: string
  fallbackUrl: string | null
  onClose: () => void
}>) {
  const { t } = useTranslation('functions')
  const attempts = useQuery(deployAttemptsQueryOptions(name))
  const latest = attempts.data?.[0]
  const failed = latest?.status === 'failed'
  const live = latest?.status === 'succeeded'
  const fns = useQuery({
    ...functionsQueryOptions('24h'),
    refetchInterval: live ? false : URL_POLL_INTERVAL_MS,
  })
  const url =
    fns.data?.functions.find((f) => f.name === name)?.url ?? fallbackUrl

  let Status = SpinnerGapIcon
  let message = t('deploy.result.building')
  if (live) {
    Status = CheckCircleIcon
    message = t('deploy.result.live', { name })
  } else if (failed) {
    Status = WarningIcon
    message = t('deploy.result.failed')
  }

  return (
    <div className="min-w-0 space-y-4" aria-live="polite">
      <h3 className="text-base font-semibold">
        {t('deploy.result.title', { name })}
      </h3>
      <p className="flex items-start gap-2 text-sm">
        <Status
          aria-hidden="true"
          className={
            live || failed
              ? 'mt-0.5 size-4 shrink-0'
              : 'mt-0.5 size-4 shrink-0 animate-spin'
          }
        />
        {message}
      </p>
      <div className="space-y-1.5">
        <p className="text-xs text-muted-foreground">
          {t('deploy.result.urlLabel')}
        </p>
        {url ? (
          <div className="flex flex-wrap items-center gap-2">
            <code className="text-sm break-all">{url}</code>
            <CopyButton text={url} />
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={<a href={url} target="_blank" rel="noreferrer" />}
            >
              {t('deploy.result.open')}
            </Button>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('deploy.result.waitingUrl')}
          </p>
        )}
      </div>
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('deploy.result.done')}
        </Button>
      </div>
    </div>
  )
}
