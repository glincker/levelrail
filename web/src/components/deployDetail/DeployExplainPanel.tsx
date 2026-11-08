import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useDeployExplanation } from '../../queries/deployExplain'

/** DeployExplainPanel says in plain words why a failed deploy failed, from its timeline. */
export function DeployExplainPanel({
  deployId,
  failed,
}: {
  deployId: string
  failed: boolean
}) {
  const { t } = useTranslation('deploySafety')
  const { data, isPending, isError } = useDeployExplanation(deployId, failed)

  if (!failed || isError) return null
  if (isPending) {
    return (
      <div role="status">
        <span className="sr-only">{t('explain.loading')}</span>
        <Skeleton decorative className="h-20 w-full" />
      </div>
    )
  }
  if (!data || !data.failed) return null

  return (
    <section
      aria-label={t('explain.title')}
      className="space-y-3 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-4"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <CircleAlertGlassIcon
          className="size-5 shrink-0 text-tone-danger"
          aria-hidden="true"
        />
        <h3 className="text-sm font-semibold text-foreground">
          {t('explain.title')}
        </h3>
        <Badge variant="destructive">
          {t(`explain.verdict.${data.verdict}`, { defaultValue: data.verdict })}
        </Badge>
        {data.rolled_back ? (
          <Badge variant="warning">{t('explain.rolledBack')}</Badge>
        ) : null}
      </div>
      <p className="text-sm font-medium text-foreground">{data.headline}</p>
      {data.cause ? (
        <p className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('explain.cause')}
          </span>{' '}
          {data.cause}
        </p>
      ) : null}
      {data.hint ? (
        <p className="text-sm text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('explain.hint')}
          </span>{' '}
          {data.hint}
        </p>
      ) : null}
      {data.evidence && data.evidence.length > 0 ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">{t('explain.evidence')}</summary>
          <ul className="space-y-1 pt-2">
            {data.evidence.map((e) => (
              <li key={e.id}>
                <span className="font-medium text-foreground">{e.title}</span>
                {e.detail || e.reason ? ` ${e.detail ?? e.reason ?? ''}` : ''}
              </li>
            ))}
          </ul>
        </details>
      ) : null}
      {data.last_logs && data.last_logs.length > 0 ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">{t('explain.lastLogs')}</summary>
          <pre className="mt-2 max-h-48 overflow-auto rounded-md bg-background/60 p-2 font-mono break-words whitespace-pre-wrap">
            {data.last_logs.join('\n')}
          </pre>
        </details>
      ) : null}
    </section>
  )
}
