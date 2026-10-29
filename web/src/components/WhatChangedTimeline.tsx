import { useTranslation } from 'react-i18next'
import {
  BellRingGlassIcon,
  CircleGlassIcon,
  GaugeGlassIcon,
  RocketGlassIcon,
  RotateCcwGlassIcon,
  SettingsGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import type { ComponentType } from 'react'
import { sortTimeline } from '../lib/timeline'
import type { TimelineEvent, TimelineKind } from '../types/investigate'

type IconType = ComponentType<{
  className?: string
  'aria-hidden'?: boolean | 'true'
}>

const KIND_ICON: Partial<Record<TimelineKind, IconType>> = {
  deploy: RocketGlassIcon,
  rollback: RotateCcwGlassIcon,
  restart: RotateCcwGlassIcon,
  saturation: GaugeGlassIcon,
  alert: BellRingGlassIcon,
  config: SettingsGlassIcon,
  env: SettingsGlassIcon,
  secret: SettingsGlassIcon,
}

const SEVERITY_TEXT = {
  info: 'text-muted-foreground',
  warning: 'text-tone-warning',
  critical: 'text-destructive',
} as const

// The merged "what changed" list around a spike: deploys, config changes,
// restarts, saturation and alerts, oldest first.
export function WhatChangedTimeline({
  events,
}: {
  events: readonly TimelineEvent[]
}) {
  const { t } = useTranslation('observability')
  const sorted = sortTimeline(events)
  if (sorted.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('investigation.noChanges')}
      </p>
    )
  }
  return (
    <ol className="space-y-2">
      {sorted.map((e, i) => {
        const Icon = KIND_ICON[e.kind] ?? CircleGlassIcon
        return (
          <li
            key={`${e.at}-${e.kind}-${i}`}
            className="flex items-start gap-2 text-sm"
          >
            <Icon
              className={`mt-0.5 size-5 shrink-0 ${SEVERITY_TEXT[e.severity]}`}
              aria-hidden="true"
            />
            <div className="min-w-0">
              <p className="text-foreground">
                {e.title}
                {e.likely_cause ? (
                  <span className="ml-2 inline-flex items-center gap-1 rounded bg-tone-warning-soft px-1.5 py-0.5 text-xs font-medium text-tone-warning">
                    <TriangleAlertGlassIcon
                      className="size-4.5 text-tone-warning"
                      aria-hidden="true"
                    />
                    {t('investigation.likelyCause')}
                  </span>
                ) : null}
              </p>
              {e.detail ? (
                <p className="text-xs break-words text-muted-foreground">
                  {e.detail}
                </p>
              ) : null}
              <time
                dateTime={e.at}
                className="text-xs text-muted-foreground tabular-nums"
              >
                {new Date(e.at).toLocaleString()}
              </time>
            </div>
          </li>
        )
      })}
    </ol>
  )
}
