import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useErrorIssue, useSetErrorStatus } from '../../queries/errorTracking'
import type {
  ErrorEventDoc,
  ErrorException,
  ErrorStatus,
} from '../../types/errorTracking'

function exceptionsOf(doc: ErrorEventDoc | undefined): ErrorException[] {
  const ex = doc?.exception
  if (!ex) return []
  return Array.isArray(ex) ? ex : (ex.values ?? [])
}

/** ErrorIssueDetail shows an issue's latest event stack and its actions. */
export function ErrorIssueDetail({
  app,
  fingerprint,
}: {
  app: string
  fingerprint: string
}) {
  const { t } = useTranslation('errors')
  const { data, isPending, error } = useErrorIssue(app, fingerprint)
  const setStatus = useSetErrorStatus(app)

  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('detail.loading')}</p>
    )
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {t('list.loadError', { message: error.message })}
        </AlertDescription>
      </Alert>
    )
  }
  const latest = data.events[0]
  const exceptions = exceptionsOf(latest?.event)

  function act(status: ErrorStatus) {
    setStatus.mutate(
      { fingerprint, status },
      {
        onSuccess: () => {
          toast.add({ title: t('detail.updated'), type: 'success' })
        },
        onError: (e) => {
          toast.add({
            title: t('detail.updateFailed'),
            description: e.message,
            type: 'error',
          })
        },
      },
    )
  }

  const status = data.issue.status
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        {status !== 'resolved' ? (
          <Button
            type="button"
            size="sm"
            disabled={setStatus.isPending}
            onClick={() => {
              act('resolved')
            }}
          >
            {t('detail.resolve')}
          </Button>
        ) : null}
        {status !== 'ignored' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={setStatus.isPending}
            onClick={() => {
              act('ignored')
            }}
          >
            {t('detail.ignore')}
          </Button>
        ) : null}
        {status !== 'open' ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={setStatus.isPending}
            onClick={() => {
              act('open')
            }}
          >
            {t('detail.reopen')}
          </Button>
        ) : null}
        <span className="text-xs text-muted-foreground">
          {t('detail.samples', { count: data.events.length })}
          {latest?.environment
            ? ` · ${t('detail.environment', { environment: latest.environment })}`
            : ''}
        </span>
      </div>
      <h4 className="text-xs font-medium text-muted-foreground">
        {t('detail.stack')}
      </h4>
      {exceptions.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('detail.noStack')}</p>
      ) : (
        exceptions.map((ex, i) => {
          const frames = [...(ex.stacktrace?.frames ?? [])].reverse()
          return (
            <div
              key={`${ex.type ?? 'exception'}-${String(i)}`}
              className="space-y-1 rounded-md bg-muted px-3 py-2"
            >
              <p className="font-mono text-xs font-medium text-foreground">
                {ex.type}
                {ex.value ? `: ${ex.value}` : ''}
              </p>
              <ol className="space-y-0.5 font-mono text-xs text-muted-foreground">
                {frames.map((f, n) => (
                  <li
                    key={`${f.filename ?? f.module ?? ''}-${f.function ?? ''}-${String(n)}`}
                    className={f.in_app ? 'text-foreground' : undefined}
                  >
                    {f.function ?? '?'} ({f.filename ?? f.module ?? '?'}
                    {f.lineno ? `:${String(f.lineno)}` : ''})
                  </li>
                ))}
              </ol>
            </div>
          )
        })
      )}
    </div>
  )
}
