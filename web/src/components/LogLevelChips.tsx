import type { ComponentType } from 'react'
import {
  BugIcon,
  InfoIcon,
  WarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'
import {
  LEVEL_BUCKETS,
  type LevelBucket,
  type LevelCounts,
} from '../lib/logLevels'

interface LevelMeta {
  label: string
  Icon: ComponentType<{
    className?: string
    'aria-hidden'?: boolean | 'true'
  }> | null
  gutter: string
}

// Row gutter colors sit on the dark log panel. Color is never the only
// signal: every colored row also carries an icon and the level word.
const LEVEL_META: Record<LevelBucket, LevelMeta> = {
  error: {
    label: 'error',
    Icon: XCircleIcon,
    gutter: 'border-red-500 text-red-400',
  },
  warn: {
    label: 'warn',
    Icon: WarningIcon,
    gutter: 'border-amber-500 text-amber-400',
  },
  info: {
    label: 'info',
    Icon: InfoIcon,
    gutter: 'border-sky-500 text-sky-400',
  },
  debug: {
    label: 'debug',
    Icon: BugIcon,
    gutter: 'border-neutral-600 text-neutral-400',
  },
  none: {
    label: 'no level',
    Icon: null,
    gutter: 'border-transparent text-neutral-600',
  },
}

export function LogLevelChips({
  counts,
  selected,
  onToggle,
}: {
  counts: LevelCounts
  selected: ReadonlySet<LevelBucket>
  onToggle: (bucket: LevelBucket) => void
}) {
  return (
    <div
      role="group"
      aria-label="Filter by log level"
      className="flex flex-wrap items-center gap-1.5"
    >
      {LEVEL_BUCKETS.filter((b) => b !== 'none' || counts.none > 0).map(
        (bucket) => {
          const { label, Icon } = LEVEL_META[bucket]
          const active = selected.has(bucket)
          return (
            <Button
              key={bucket}
              type="button"
              size="sm"
              variant={active ? 'default' : 'outline'}
              aria-pressed={active}
              disabled={counts[bucket] === 0 && !active}
              onClick={() => {
                onToggle(bucket)
              }}
            >
              {Icon ? <Icon className="size-3.5" aria-hidden="true" /> : null}
              {label}
              <span className="tabular-nums opacity-80">
                {counts[bucket].toLocaleString()}
              </span>
            </Button>
          )
        },
      )}
    </div>
  )
}

export function LogLevelGutter({
  level,
  bucket,
}: {
  level: string | undefined
  bucket: LevelBucket
}) {
  const { Icon, gutter } = LEVEL_META[bucket]
  return (
    <span
      className={`flex w-16 shrink-0 items-center gap-1 self-stretch border-l-2 pl-1.5 text-[0.65rem] uppercase ${gutter}`}
    >
      {Icon ? <Icon className="size-3" aria-hidden="true" /> : null}
      {level ?? ''}
    </span>
  )
}
