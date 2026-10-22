import { useTranslation } from 'react-i18next'
import { barWidthClass, formatMs } from '../../lib/deployEvents'
import type { DeployEventPhase } from '../../types/deployEvents'

export function PhaseSummary({ phases }: { phases: DeployEventPhase[] }) {
  const { t } = useTranslation('deployEvents')
  const max = Math.max(0, ...phases.map((p) => p.duration_ms))
  if (phases.length === 0) return null
  return (
    <section aria-label={t('phases.label')} className="rounded-xl border p-4">
      <h2 className="text-sm font-medium">{t('phases.title')}</h2>
      <ul className="mt-3 flex flex-col gap-2">
        {phases.map((p) => (
          <li
            key={p.phase}
            className="grid grid-cols-[6rem_1fr_4.5rem] items-center gap-3 text-sm"
          >
            <span className="truncate text-muted-foreground">
              {t(`phases.${p.phase}` as 'phases.build', {
                defaultValue: p.phase,
              })}
            </span>
            <span aria-hidden="true" className="h-2 rounded-full bg-muted">
              <span
                className={`block h-2 rounded-full bg-tone-info-solid ${barWidthClass(p.duration_ms, max)}`}
              />
            </span>
            <span className="text-right tabular-nums">
              {formatMs(p.duration_ms)}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
