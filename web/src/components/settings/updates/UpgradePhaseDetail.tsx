import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { SelfUpgradeLogLine } from '../../../queries/selfUpgrade'
import type { PhaseView, StepView } from '../../../lib/upgradePhases'
import { formatElapsed } from '../../../lib/upgradeTime'
import { scaleClass } from './progressScale'

function StepIcon({
  state,
  motion,
}: {
  state: StepView['state']
  motion: boolean
}) {
  const cls = 'size-4 shrink-0'
  if (state === 'running') {
    return (
      <LoaderCircleGlassIcon
        size="1.4em"
        className={cn(cls, 'text-primary', motion && 'animate-spin')}
        aria-hidden
      />
    )
  }
  if (state === 'ok')
    return (
      <CircleCheckGlassIcon
        size="1.4em"
        className={cn(cls, 'text-success')}
        aria-hidden
      />
    )
  if (state === 'failed')
    return (
      <CircleXGlassIcon
        size="1.4em"
        className={cn(cls, 'text-destructive')}
        aria-hidden
      />
    )
  if (state === 'skipped')
    return (
      <CircleMinusGlassIcon
        size="1.4em"
        className={cn(cls, 'text-muted-foreground')}
        aria-hidden
      />
    )
  return (
    <CircleGlassIcon
      size="1.4em"
      className={cn(cls, 'text-muted-foreground')}
      aria-hidden
    />
  )
}

interface Props {
  id: string
  phase: PhaseView
  log: SelfUpgradeLogLine[]
  error: string
  now: number
  motion: boolean
  percent: number | null
}

export function UpgradePhaseDetail({
  id,
  phase,
  log,
  error,
  now,
  motion,
  percent,
}: Props) {
  const { t } = useTranslation('updates')
  const names = new Set(phase.steps.map((s) => s.name))
  const lines = log.filter((l) => names.has(l.step))
  const running = phase.state === 'running'
  const indeterminate = percent === null
  return (
    <div
      id={id}
      role="region"
      aria-label={t(`pipeline.phase.${phase.id}`)}
      className="space-y-3 rounded-md border border-border p-3"
    >
      {running ? (
        <div
          className="relative h-1.5 overflow-hidden rounded-full bg-muted"
          aria-hidden
        >
          <div
            className={cn(
              'h-full w-full origin-left rounded-full bg-primary/80 transition-transform duration-500',
              indeterminate
                ? cn(
                    'bg-[repeating-linear-gradient(45deg,var(--primary)_0_8px,transparent_8px_14px)]',
                    motion && 'upg-stripe',
                  )
                : scaleClass(percent),
            )}
          />
          {motion ? (
            <div className="upg-sheen absolute inset-y-0 left-0 w-1/4 bg-gradient-to-r from-transparent via-white/50 to-transparent" />
          ) : null}
        </div>
      ) : null}
      <ol className="space-y-1.5">
        {phase.steps.map((s) => (
          <li
            key={s.name}
            className="flex flex-wrap items-center gap-x-2 text-sm"
          >
            <StepIcon state={s.state} motion={motion} />
            <span
              className={cn(
                s.state === 'pending' && 'text-muted-foreground',
                s.state === 'failed' && 'text-destructive',
              )}
            >
              {t(`selfUpgrade.step.${s.name}`, { defaultValue: s.name })}
            </span>
            <span className="sr-only">{t(`pipeline.state.${s.state}`)}</span>
            {s.startedAt ? (
              <span className="text-xs text-muted-foreground">
                {new Date(s.startedAt).toLocaleTimeString()}
              </span>
            ) : null}
            {s.state === 'running' && s.startedAt ? (
              <span className="font-mono text-xs text-primary">
                {formatElapsed(now - s.startedAt)}
              </span>
            ) : s.durationMs > 0 ? (
              <span className="font-mono text-xs text-muted-foreground">
                {formatElapsed(s.durationMs)}
              </span>
            ) : null}
            {s.detail ? (
              <span className="min-w-0 basis-full truncate pl-6 text-xs text-muted-foreground sm:flex-1 sm:basis-auto sm:pl-0">
                {s.detail}
              </span>
            ) : null}
          </li>
        ))}
      </ol>
      {phase.state === 'failed' && error ? (
        <p className="break-words rounded-md bg-destructive/10 p-2 text-sm text-destructive">
          {error}
        </p>
      ) : null}
      {lines.length > 0 ? (
        <pre className="max-h-44 overflow-auto rounded-md bg-muted p-2 font-mono text-xs leading-relaxed">
          {lines
            .map((l) => `${l.at.slice(11, 19)} [${l.step}] ${l.line}`)
            .join('\n')}
        </pre>
      ) : null}
    </div>
  )
}
