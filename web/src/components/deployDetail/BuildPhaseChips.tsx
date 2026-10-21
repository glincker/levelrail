import type { BuildLogAnalysis, LogPhase } from '../../lib/buildLogPhases'
import { hasRecognisedPhases } from '../../lib/buildLogPhases'
import { useDeployDetailT } from './useDeployDetailT'

export type PhaseFilter = 'all' | Exclude<LogPhase, 'other'>

const CHIP_PHASES: Exclude<LogPhase, 'other'>[] = ['install', 'build', 'push']

/** Filter chips over the build log; hidden when no phase is recognised. */
export function BuildPhaseChips({
  analysis,
  active,
  onSelect,
}: {
  analysis: BuildLogAnalysis
  active: PhaseFilter
  onSelect: (next: PhaseFilter) => void
}) {
  const { t } = useDeployDetailT()
  if (!hasRecognisedPhases(analysis)) return null

  const options: { key: PhaseFilter; count: number }[] = [
    { key: 'all', count: analysis.linePhases.length },
    ...CHIP_PHASES.map((key) => ({ key, count: analysis.counts[key] })),
  ]

  return (
    <div
      role="group"
      aria-label={t('phases.label')}
      className="flex flex-wrap items-center gap-2"
    >
      {options.map(({ key, count }) => (
        <button
          key={key}
          type="button"
          aria-pressed={active === key}
          disabled={key !== 'all' && count === 0}
          onClick={() => {
            onSelect(key)
          }}
          className={`rounded-full border px-3 py-1 text-xs font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-50 ${
            active === key
              ? 'border-foreground bg-foreground text-background'
              : 'border-border text-muted-foreground hover:bg-muted'
          }`}
        >
          {t('phases.chip', { phase: t(`phases.${key}`), count })}
        </button>
      ))}
    </div>
  )
}
