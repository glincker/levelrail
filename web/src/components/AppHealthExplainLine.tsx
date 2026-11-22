import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useStopApp } from '../queries/apps'
import { useAppStatus } from '../queries/appStatus'
import { useAppHealthExplain } from '../queries/healthExplain'

// One compact line under the health summary: the last failing check, restarts,
// the newest error line and the suggested next step. Hidden when the app is healthy.
export function AppHealthExplainLine({ appName }: { appName: string }) {
  const { t } = useTranslation('common')
  const { data } = useAppHealthExplain(appName)
  const { data: status } = useAppStatus(appName)
  const stopApp = useStopApp()
  const backoff = status?.backoff
  if (!data || data.state === 'healthy') return null

  const parts: string[] = []
  if (data.failing_check) {
    parts.push(
      t('healthExplain.failingCheck', {
        target: data.failing_check.target,
        since: new Date(data.failing_check.since).toLocaleString(),
      }),
    )
  }
  if (data.crashloop) parts.push(t('healthExplain.crashloop'))
  if (data.restarts)
    parts.push(t('healthExplain.restarts', { n: data.restarts }))
  if (data.last_deploy && data.last_deploy.status === 'failed') {
    parts.push(
      t('healthExplain.lastDeploy', { status: data.last_deploy.status }),
    )
  }
  if (data.log_error)
    parts.push(t('healthExplain.logError', { line: data.log_error }))
  const next = data.next_step
  const nextText =
    next.kind === 'none'
      ? null
      : t(`healthExplain.next.${next.kind}`, { preset: next.preset ?? '' })

  return (
    <p
      className="mb-2 break-words text-sm text-foreground"
      data-testid="health-explain-line"
    >
      <span className="font-medium">{t('healthExplain.label')}: </span>
      {parts.join(' · ')}
      {backoff ? (
        <span className="ml-2 inline-flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground" data-testid="restart-backoff">
            {t('healthExplain.backoff', {
              n: backoff.restarts,
              seconds: backoff.retry_in_seconds,
              at: new Date(backoff.next_restart_at).toLocaleTimeString(),
            })}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={stopApp.isPending}
            onClick={() => {
              stopApp.mutate(appName, {
                onError: (error) => {
                  toast.add({ title: error.message, type: 'error' })
                },
              })
            }}
          >
            {t('healthExplain.stopRetrying')}
          </Button>
        </span>
      ) : null}
      {nextText ? (
        <span className="text-muted-foreground">
          {' → '}
          {nextText}
          {next.command ? <code className="ml-1">{next.command}</code> : null}
        </span>
      ) : null}
    </p>
  )
}
