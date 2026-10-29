import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { useProbeAssetBucket } from '../../queries/assets'
import { corsRuleJson } from '../../lib/assetsFormat'

export function ProbePanel({ bucketId }: Readonly<{ bucketId: string }>) {
  const { t } = useTranslation('assets')
  const probe = useProbeAssetBucket(bucketId)
  const result = probe.data
  const manual = result?.cors?.status === 'manual'

  return (
    <section
      aria-labelledby="asset-probe-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="asset-probe-title" className="text-sm font-medium">
          {t('probe.title')}
        </h2>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={probe.isPending}
          onClick={() => {
            probe.mutate(false)
          }}
        >
          {probe.isPending
            ? t('probe.running')
            : result
              ? t('probe.recheck')
              : t('probe.run')}
        </Button>
      </div>
      {probe.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {probe.error.message}
        </p>
      ) : null}
      {result ? (
        <div className="space-y-2 text-sm" aria-live="polite">
          <p className="flex items-center gap-1.5">
            {result.ok ? (
              <CircleCheckGlassIcon
                aria-hidden="true"
                className="size-5 text-tone-success"
              />
            ) : (
              <CircleXGlassIcon
                aria-hidden="true"
                className="size-5 text-destructive"
              />
            )}
            {result.ok ? t('probe.ok') : (result.message ?? t('probe.failed'))}
          </p>
          <ul className="space-y-0.5 text-xs text-muted-foreground">
            {result.steps.map((s) => (
              <li key={s.name}>
                {s.name}: {s.ok ? t('probe.stepOk') : t('probe.stepFail')}
                {s.error ? ` (${s.error})` : ''}
              </li>
            ))}
          </ul>
          {result.cors?.status === 'applied' ? (
            <p className="text-xs text-tone-success">
              {t('probe.corsApplied')}
            </p>
          ) : null}
          {result.warnings.map((w) => (
            <div
              key={w.code}
              className="flex items-start gap-2 rounded-md border border-tone-warning-border bg-tone-warning-soft p-2 text-xs"
            >
              <TriangleAlertGlassIcon
                aria-hidden="true"
                className="mt-0.5 size-6 shrink-0 text-tone-warning"
              />
              <p className="min-w-0 flex-1">{w.message}</p>
              {w.fix === 'fix_cors' ? (
                <Button
                  type="button"
                  size="xs"
                  disabled={probe.isPending}
                  onClick={() => {
                    probe.mutate(true)
                  }}
                >
                  {probe.isPending ? t('probe.fixingCors') : t('probe.fixCors')}
                </Button>
              ) : null}
            </div>
          ))}
          {manual && result.cors ? (
            <div className="space-y-1.5">
              <p className="text-xs font-medium">{t('probe.manualTitle')}</p>
              <p className="text-xs text-muted-foreground">
                {t('probe.manualBody')}
              </p>
              <CodeBlock
                code={corsRuleJson(result.cors.rule)}
                language="json"
                maxHeight="sm"
              />
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}
