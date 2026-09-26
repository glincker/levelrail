import {
  CheckCircleIcon,
  CircleNotchIcon,
  PauseCircleIcon,
  RocketLaunchIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { ReactNode } from 'react'
import type { Tone } from '@/components/kit'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { ActivityEntry } from '../../lib/fleetStats'

const ACTIVITY_SHOWN = 6

const STATUS_VIEW: Record<
  DeployAttempt['status'],
  { tone: Tone; verb: string; icon: ReactNode }
> = {
  succeeded: {
    tone: 'success',
    verb: 'deployed',
    icon: <CheckCircleIcon weight="fill" className="size-4" />,
  },
  failed: {
    tone: 'danger',
    verb: 'deploy failed',
    icon: <XCircleIcon weight="fill" className="size-4" />,
  },
  running: {
    tone: 'info',
    verb: 'deploying',
    icon: (
      <CircleNotchIcon className="size-4 animate-spin motion-reduce:animate-none" />
    ),
  },
  held: {
    tone: 'warning',
    verb: 'deploy held',
    icon: <PauseCircleIcon className="size-4" />,
  },
  superseded: {
    tone: 'neutral',
    verb: 'superseded',
    icon: <RocketLaunchIcon className="size-4" />,
  },
  queued: {
    tone: 'neutral',
    verb: 'deploy queued',
    icon: <PauseCircleIcon className="size-4" />,
  },
  canceled: {
    tone: 'neutral',
    verb: 'deploy canceled',
    icon: <XCircleIcon className="size-4" />,
  },
}

export function toTimelineItems(
  entries: ActivityEntry[],
  onOpen: (app: string) => void,
) {
  return entries.slice(0, ACTIVITY_SHOWN).map((e) => {
    const view = STATUS_VIEW[e.status]
    return {
      id: e.id,
      at: e.at,
      icon: view.icon,
      tone: view.tone,
      title: `${e.app} ${view.verb}`,
      detail: e.detail,
      onClick: () => {
        onOpen(e.app)
      },
    }
  })
}
