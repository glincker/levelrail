import { useTranslation } from 'react-i18next'
import { formatSize } from '../lib/format'
import type { PipelineRun } from '../types/pipelines'

export function PipelineRunMeta({ run }: { run: PipelineRun }) {
  const { t } = useTranslation('pipelines')
  const params = Object.entries(run.inputs ?? {}).sort(([a], [b]) =>
    a.localeCompare(b),
  )
  return (
    <section aria-labelledby="run-meta" className="space-y-2">
      <h3 id="run-meta" className="text-sm font-semibold text-foreground">
        {t('meta.title')}
      </h3>
      <dl className="grid grid-cols-1 gap-x-6 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
        <dt className="text-muted-foreground">{t('meta.source')}</dt>
        <dd className="text-foreground">
          {t(`trigger.${run.trigger}`, { defaultValue: run.trigger })}
          {run.actor ? ` (${run.actor})` : ''}
        </dd>
        {run.ref ? (
          <>
            <dt className="text-muted-foreground">{t('meta.ref')}</dt>
            <dd className="break-all font-mono text-xs text-foreground">
              {run.ref}
            </dd>
          </>
        ) : null}
        {run.commit_sha ? (
          <>
            <dt className="text-muted-foreground">{t('meta.commit')}</dt>
            <dd className="font-mono text-xs text-foreground">
              {run.commit_sha.slice(0, 12)}
            </dd>
          </>
        ) : null}
        <dt className="text-muted-foreground">{t('meta.parameters')}</dt>
        <dd>
          {params.length === 0 ? (
            <span className="text-muted-foreground">
              {t('meta.noParameters')}
            </span>
          ) : (
            <ul className="space-y-0.5">
              {params.map(([k, v]) => (
                <li key={k} className="break-all font-mono text-xs">
                  <span className="text-muted-foreground">{k}=</span>
                  <span className="text-foreground">{v}</span>
                </li>
              ))}
            </ul>
          )}
        </dd>
        {run.payload_bytes ? (
          <>
            <dt className="text-muted-foreground">{t('meta.payload')}</dt>
            <dd className="text-foreground">
              {t('meta.payloadValue', {
                size: formatSize(run.payload_bytes),
                type: run.payload_type ?? '',
              })}
            </dd>
          </>
        ) : null}
      </dl>
    </section>
  )
}
