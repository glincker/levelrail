import {
  ArrowArcLeftIcon,
  ArrowClockwiseIcon,
  ArrowsOutIcon,
  GearIcon,
  KeyIcon,
  PauseIcon,
  PlayIcon,
  RocketLaunchIcon,
  SnowflakeIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Tone } from '@/components/kit'
import type {
  TimelineEntry,
  TimelineKind,
  TimelineStatus,
} from '../../queries/appTimeline'

export const TIMELINE_MAX_ITEMS = 8

const KIND_ICON: Record<TimelineKind, React.ReactNode> = {
  deploy: <RocketLaunchIcon />,
  rollback: <ArrowArcLeftIcon />,
  restart: <ArrowClockwiseIcon />,
  env_change: <KeyIcon />,
  secret_change: <KeyIcon />,
  config_change: <GearIcon />,
  scale: <ArrowsOutIcon />,
  freeze_override: <SnowflakeIcon />,
  suspend: <PauseIcon />,
  resume: <PlayIcon />,
}

const STATUS_TONE: Record<TimelineStatus, Tone> = {
  succeeded: 'success',
  failed: 'danger',
  in_progress: 'info',
  pending: 'warning',
  info: 'neutral',
}

export function toTimelineItems(
  entries: TimelineEntry[],
  onOpenDeploy: (deployId: string) => void,
) {
  return entries.slice(0, TIMELINE_MAX_ITEMS).map((e) => {
    const deployId = e.ref?.type === 'deploy_attempt' ? e.ref.id : undefined
    return {
      id: e.id,
      at: e.at,
      icon: KIND_ICON[e.kind],
      tone: STATUS_TONE[e.status],
      title: e.title,
      detail: e.detail,
      actor: e.actor,
      onClick: deployId ? () => onOpenDeploy(deployId) : undefined,
    }
  })
}
