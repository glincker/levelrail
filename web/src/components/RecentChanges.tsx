import { useMemo, type ReactNode } from 'react'
import {
  ArrowUUpLeftIcon,
  ArrowsOutIcon,
  GlobeIcon,
  KeyIcon,
  PowerIcon,
  RocketLaunchIcon,
  ShareNetworkIcon,
  SlidersHorizontalIcon,
  SnowflakeIcon,
  WrenchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { InfoTip, StatusPill, Timeline } from '@/components/kit'
import type { TimelineItem, Tone } from '@/components/kit'
import { useAppChanges, type AppChange } from '../queries/appChanges'

const KIND_ICON: Record<AppChange['kind'], ReactNode> = {
  deploy: <RocketLaunchIcon />,
  rollback: <ArrowUUpLeftIcon />,
  config: <SlidersHorizontalIcon />,
  env: <KeyIcon />,
  secret: <KeyIcon />,
  domain: <GlobeIcon />,
  scale: <ArrowsOutIcon />,
  loadbalancer: <ShareNetworkIcon />,
  freeze: <SnowflakeIcon />,
  maintenance: <WrenchIcon />,
  lifecycle: <PowerIcon />,
}

function kindTone(c: AppChange): Tone {
  if (c.likely_cause) return 'warning'
  if (c.title.endsWith('failed')) return 'danger'
  return 'neutral'
}

export function LikelyCause() {
  return (
    <span className="inline-flex items-center gap-0.5">
      <StatusPill tone="warning" size="sm" label="Likely cause" />
      <InfoTip label="About likely cause">
        A heuristic, not a diagnosis: the most recent change that took effect
        before the alert (a deploy, rollback, config, env, domain, scaling or
        load balancer change). Restarts and maintenance are never picked.
        Something earlier, or something outside this platform, can still be the
        real cause.
      </InfoTip>
    </span>
  )
}

function toTimelineItems(changes: AppChange[]): TimelineItem[] {
  return changes.map((c, i) => ({
    id: `${c.ref ?? c.kind}-${c.at}-${i}`,
    at: c.at,
    icon: KIND_ICON[c.kind],
    tone: kindTone(c),
    title: c.title,
    detail:
      [c.keys?.length ? c.keys.join(', ') : '', c.detail ?? '']
        .filter(Boolean)
        .join(' - ') || undefined,
    actor: c.actor || undefined,
    badge: c.likely_cause ? <LikelyCause /> : undefined,
  }))
}

// What changed on an app in the window before `until` (empty means now).
export function RecentChanges({
  app,
  until = '',
  className,
}: {
  app: string
  until?: string
  className?: string
}) {
  const { data, isLoading, isError } = useAppChanges(app, until)
  const items = useMemo(() => toTimelineItems(data?.changes ?? []), [data])
  if (!isLoading && (isError || data === null)) return null

  const minutes = data ? Math.round(data.window_seconds / 60) : 30
  const more = data ? data.total - data.changes.length : 0
  return (
    <div className={className} data-testid="recent-changes">
      <h3 className="mb-2 flex items-center gap-1 text-xs font-semibold text-foreground">
        What changed in the last {minutes} minutes
        <InfoTip label="About recent changes">
          Deploys (with digest and who), rollbacks, config, domain, scaling and
          load balancer changes, and env or secret key names. Values are never
          shown.
        </InfoTip>
      </h3>
      <Timeline
        items={items}
        loading={isLoading}
        emptyLabel="Nothing changed on this app in that window."
      />
      {more > 0 ? (
        <p className="mt-2 text-xs text-muted-foreground">and {more} more</p>
      ) : null}
    </div>
  )
}
