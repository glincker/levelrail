import { useState } from 'react'
import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import {
  formatStepDuration,
  hasRecognisedPhases,
  type BuildLogAnalysis,
  type BuildStep,
  type LogPhase,
} from '../../lib/buildLogPhases'
import { useDeployDetailT } from './useDeployDetailT'

const TOTAL_PHASES: Exclude<LogPhase, 'other'>[] = ['install', 'build', 'push']

/** Collapsed list of parsed build steps with per-phase totals. */
export function BuildStepList({
  analysis,
  onJump,
}: {
  analysis: BuildLogAnalysis
  onJump: (step: BuildStep) => void
}) {
  const { t } = useDeployDetailT()
  const [open, setOpen] = useState(false)
  if (!hasRecognisedPhases(analysis) || analysis.steps.length === 0) {
    return null
  }
  const Caret = open ? CaretDownIcon : CaretRightIcon
  const steps = [...analysis.steps].sort((a, b) => a.firstLine - b.firstLine)

  return (
    <section
      aria-label={t('phases.stepsLabel')}
      className="rounded-lg border border-border px-3 py-2"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <button
          type="button"
          aria-expanded={open}
          onClick={() => {
            setOpen((prev) => !prev)
          }}
          className="flex items-center gap-1.5 text-sm font-medium text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <Caret className="size-3.5" aria-hidden="true" />
          {t(open ? 'phases.hideSteps' : 'phases.showSteps')}
        </button>
        <ul className="flex flex-wrap gap-3 text-xs text-muted-foreground">
          {TOTAL_PHASES.map((phase) => (
            <li key={phase} data-testid={`phase-total-${phase}`}>
              {t('phases.total', {
                phase: t(`phases.${phase}`),
                time:
                  analysis.durations[phase] > 0
                    ? formatStepDuration(analysis.durations[phase])
                    : t('phases.noTiming'),
              })}
            </li>
          ))}
        </ul>
      </div>
      {open ? (
        <ol className="mt-2 space-y-1">
          {steps.map((step) => (
            <li key={step.id}>
              <button
                type="button"
                title={t('phases.jump', { label: step.label })}
                onClick={() => {
                  onJump(step)
                }}
                className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                <span className="min-w-0 flex-1 truncate font-mono text-foreground">
                  {step.label}
                </span>
                <span className="rounded border border-border px-1.5 py-0.5 text-muted-foreground">
                  {t(`phases.${step.phase}`)}
                </span>
                {step.cached ? (
                  <span className="rounded bg-muted px-1.5 py-0.5 text-foreground">
                    {t('phases.cached')}
                  </span>
                ) : null}
                <span className="w-14 shrink-0 text-right tabular-nums text-muted-foreground">
                  {step.durationSec !== undefined
                    ? formatStepDuration(step.durationSec)
                    : ''}
                </span>
              </button>
            </li>
          ))}
        </ol>
      ) : null}
    </section>
  )
}
