import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import type { DeployStage } from '../lib/deployStages'
import { formatDeployDuration } from '../lib/deployDuration'
import { cn } from '@/lib/utils'

export interface StageTab {
  id: string
  label: string
  stage?: DeployStage
  hint?: string
}

function StageIcon({ stage }: { stage?: DeployStage }) {
  const cls = 'size-4 shrink-0'
  switch (stage?.status) {
    case 'done':
      return (
        <CircleCheckGlassIcon
          size="1.2em"
          className={cn(cls, 'text-success')}
        />
      )
    case 'failed':
      return (
        <CircleAlertGlassIcon
          size="1.2em"
          className={cn(cls, 'text-tone-danger')}
        />
      )
    case 'running':
      return (
        <LoaderCircleGlassIcon
          size="1.2em"
          className={cn(
            cls,
            'animate-spin text-info motion-reduce:animate-none',
          )}
        />
      )
    default:
      return (
        <CircleDashedGlassIcon
          size="1.2em"
          className={cn(cls, 'text-muted-foreground')}
        />
      )
  }
}

export function DeployStageTabs({
  tabs,
  value,
  onChange,
}: {
  tabs: StageTab[]
  value: string
  onChange: (id: string) => void
}) {
  return (
    <div
      role="tablist"
      aria-label="Deploy sections"
      className="flex gap-1 overflow-x-auto border-b border-border"
    >
      {tabs.map((t) => {
        const active = t.id === value
        const duration =
          t.stage?.startedAt && t.stage.status !== 'pending'
            ? formatDeployDuration(t.stage.startedAt, t.stage.finishedAt)
            : ''
        return (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => {
              onChange(t.id)
            }}
            className={cn(
              '-mb-px flex shrink-0 items-center gap-2 border-b-2 px-3 py-2 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60',
              active
                ? 'border-foreground text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {t.stage ? <StageIcon stage={t.stage} /> : null}
            {t.label}
            {duration ? (
              <span className="font-mono text-xs font-normal text-muted-foreground">
                {duration}
              </span>
            ) : null}
            {t.hint ? (
              <span className="rounded-full bg-muted px-1.5 text-xs font-normal text-muted-foreground">
                {t.hint}
              </span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
