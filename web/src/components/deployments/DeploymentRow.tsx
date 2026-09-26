import {
  ArrowUUpLeftIcon,
  ArrowsClockwiseIcon,
  GitBranchIcon,
  GitCommitIcon,
} from '@phosphor-icons/react/dist/ssr'
import { RelativeTime, StatusPill } from '@/components/kit'
import { cn } from '@/lib/utils'
import type { Deployment } from '../../types/deployment'
import {
  durationLabel,
  headline,
  initials,
  isProduction,
  isRollback,
  rowKind,
  shortSha,
  statusView,
  stepProgress,
  subtitle,
} from '../../lib/deploymentPresentation'

export function StatusCell({ d, now }: { d: Deployment; now: number }) {
  const v = statusView(d.status)
  const Icon = v.icon
  const dur = durationLabel(d, now)
  const steps = d.status === 'building' ? stepProgress(d) : ''
  return (
    <span className="flex shrink-0 items-center gap-2">
      <StatusPill
        size="sm"
        tone={v.tone}
        label={v.label}
        icon={
          <Icon
            className={cn(
              'size-3.5',
              v.spin && 'animate-spin motion-reduce:animate-none',
            )}
            weight="bold"
            aria-hidden="true"
          />
        }
      />
      {dur && (
        <span className="font-mono text-xs text-muted-foreground tabular-nums">
          {dur}
        </span>
      )}
      {steps && <span className="text-xs text-muted-foreground">{steps}</span>}
    </span>
  )
}

export function EnvPill({ d }: { d: Deployment }) {
  if (!d.environment) return null
  const current = d.is_live && isProduction(d.environment)
  return (
    <span
      title={current ? 'Current production release' : d.environment}
      className={cn(
        'inline-flex h-5 shrink-0 items-center rounded-full border px-2 text-[11px] font-medium capitalize',
        current
          ? 'border-tone-accent-border bg-tone-accent-soft text-tone-accent'
          : 'border-border text-muted-foreground',
      )}
    >
      {d.environment}
      {current && <span className="sr-only"> (current release)</span>}
    </span>
  )
}

function Author({ name }: { name: string }) {
  if (!name) return null
  return (
    <span
      title={name}
      className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-medium text-muted-foreground"
    >
      <span aria-hidden="true">{initials(name)}</span>
      <span className="sr-only">{name}</span>
    </span>
  )
}

export interface DeploymentRowProps {
  d: Deployment
  now: number
  open: boolean
  focused: boolean
  onOpen: (id: string) => void
}

export function DeploymentRow({
  d,
  now,
  open,
  focused,
  onOpen,
}: DeploymentRowProps) {
  const kind = rowKind(d)
  const sub = subtitle(d)
  const rollback = isRollback(d)
  const muted = kind === 'superseded' || d.status === 'canceled'
  const Lead = rollback ? ArrowUUpLeftIcon : null
  return (
    <button
      type="button"
      data-deployment-id={d.id}
      data-kind={kind}
      aria-current={open ? 'true' : undefined}
      onClick={() => {
        onOpen(d.id)
      }}
      className={cn(
        'flex w-full cursor-pointer flex-col gap-1 border-b border-border px-3 py-2.5 text-left outline-none transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-inset md:flex-row md:items-center md:gap-3',
        open && 'bg-muted/60',
        focused && 'ring-2 ring-ring/60 ring-inset',
        muted && 'opacity-70',
      )}
    >
      <span className="flex min-w-0 items-center gap-2 md:contents">
        <span className="flex min-w-0 flex-1 flex-col">
          <span className="flex min-w-0 items-center gap-1.5 text-sm font-medium">
            {Lead ? (
              <Lead
                className="size-3.5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            ) : kind === 'redeploy' ? (
              <ArrowsClockwiseIcon
                className="size-3.5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            ) : null}
            <span className="truncate" title={headline(d)}>
              {headline(d)}
            </span>
          </span>
          {sub && (
            <span
              className={cn(
                'truncate text-xs',
                d.status === 'failed'
                  ? 'text-tone-danger'
                  : 'text-muted-foreground',
              )}
              title={sub}
            >
              {sub}
            </span>
          )}
        </span>
        <StatusCell d={d} now={now} />
      </span>
      <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground md:contents">
        <span className="md:w-24">
          <EnvPill d={d} />
        </span>
        <span className="max-w-40 truncate font-medium text-foreground md:w-32">
          {d.app}
        </span>
        <span className="flex items-center gap-1 font-mono md:w-20">
          {!rollback && d.commit_sha && (
            <>
              <GitCommitIcon className="size-3.5" aria-hidden="true" />
              {shortSha(d.commit_sha)}
            </>
          )}
        </span>
        <span className="flex max-w-32 items-center gap-1 truncate font-mono md:w-28">
          {!rollback && d.branch && (
            <>
              <GitBranchIcon className="size-3.5 shrink-0" aria-hidden="true" />
              <span className="truncate">{d.branch}</span>
            </>
          )}
        </span>
        <span className="md:w-20 md:text-right">
          <RelativeTime at={d.started_at} live />
        </span>
        <Author name={d.author} />
      </span>
    </button>
  )
}
