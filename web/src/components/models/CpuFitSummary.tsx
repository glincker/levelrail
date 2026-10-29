import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { formatGiB, speedRange, verdictTone } from '../../lib/modelsCpu'
import type { CpuFitReport } from '../../types/modelsCpu'

// Live preflight for the chosen model and caps: verdict, why, the
// arithmetic, the speed range (always an estimate) and a smaller model to
// try when it will not fit.
export function CpuFitSummary({
  fit,
  onPickSuggested,
}: {
  fit: CpuFitReport
  onPickSuggested: (id: string) => void
}) {
  const { t } = useTranslation('models')
  const range = speedRange(fit.tokens_per_sec_low, fit.tokens_per_sec_high)
  return (
    <div
      className="space-y-2 rounded-lg border border-border bg-card p-3 text-sm"
      aria-live="polite"
    >
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill
          tone={verdictTone(fit.verdict)}
          label={t(`cpu.verdict.${fit.verdict}`)}
        />
        <span className="text-muted-foreground">{fit.reason}</span>
      </div>
      {fit.arithmetic ? (
        <p className="font-mono text-xs text-muted-foreground">
          {fit.arithmetic}
        </p>
      ) : null}
      {range ? (
        <p className="text-xs text-muted-foreground">
          {t('cpu.fit.speed', { range })}
        </p>
      ) : null}
      {fit.disk_bytes > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('cpu.fit.disk', { size: formatGiB(fit.disk_bytes) })}
        </p>
      ) : null}
      {fit.warnings.map((w) => (
        <p key={w} className="flex gap-1.5 text-xs text-tone-warning">
          <TriangleAlertGlassIcon
            className="mt-0.5 size-5 shrink-0 text-tone-warning"
            aria-hidden="true"
          />
          {w}
        </p>
      ))}
      {fit.suggestion ? (
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">{fit.suggestion}</span>
          {fit.suggested_id ? (
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => {
                onPickSuggested(fit.suggested_id ?? '')
              }}
            >
              {t('cpu.fit.useSuggested')}
            </Button>
          ) : null}
        </div>
      ) : null}
      <p className="text-[11px] text-muted-foreground">{fit.note}</p>
    </div>
  )
}
