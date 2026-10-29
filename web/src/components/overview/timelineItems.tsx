import {
  KeyGlassIcon,
  MaximizeGlassIcon,
  MoonGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
  SettingsGlassIcon,
  ShieldAlertGlassIcon,
  SnowflakeGlassIcon,
  SunGlassIcon,
  UndoGlassIcon,
} from '@thesvg/ui-react'
import type { Tone } from '@/components/kit'
import type {
  TimelineEntry,
  TimelineKind,
  TimelineStatus,
} from '../../queries/appTimeline'

export const TIMELINE_MAX_ITEMS = 8

const KIND_ICON: Record<TimelineKind, React.ReactNode> = {
  deploy: <RocketGlassIcon size="1.4em" />,
  rollback: <UndoGlassIcon size="1.4em" />,
  restart: <RotateCwGlassIcon size="1.4em" />,
  env_change: <KeyGlassIcon size="1.4em" />,
  secret_change: <KeyGlassIcon size="1.4em" />,
  config_change: <SettingsGlassIcon size="1.4em" />,
  scale: <MaximizeGlassIcon size="1.4em" />,
  freeze_override: <SnowflakeGlassIcon size="1.4em" />,
  scan_gate_override: (
    <ShieldAlertGlassIcon className="text-tone-warning" size="1.4em" />
  ),
  suspend: <PauseGlassIcon size="1.4em" />,
  resume: <PlayGlassIcon size="1.4em" />,
  slept: <MoonGlassIcon size="1.4em" />,
  woke: <SunGlassIcon size="1.4em" />,
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
