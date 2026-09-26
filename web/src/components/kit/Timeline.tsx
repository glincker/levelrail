import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { RelativeTime } from './RelativeTime'
import { SkeletonList } from './Skeleton'
import { TONE, type Tone } from './tone'

export interface TimelineItem {
  id: string
  at: string | Date
  icon: ReactNode
  tone?: Tone
  title: string
  detail?: string
  actor?: string
  onClick?: () => void
}

export interface TimelineProps {
  items: TimelineItem[]
  loading?: boolean
  emptyLabel?: string
}

function Row({ item, last }: { item: TimelineItem; last: boolean }) {
  const t = TONE[item.tone ?? 'neutral']
  const content = (
    <>
      <div className="flex min-w-0 flex-col">
        <span className="text-sm font-medium">{item.title}</span>
        {item.detail && (
          <span className="text-sm text-muted-foreground">{item.detail}</span>
        )}
        <span className="text-xs text-muted-foreground">
          {item.actor && <>{item.actor} &middot; </>}
          <RelativeTime at={item.at} live />
        </span>
      </div>
    </>
  )
  return (
    <li className="kit-enter relative flex gap-3 pb-4 last:pb-0">
      {!last && (
        <span
          aria-hidden="true"
          className="absolute top-8 bottom-0 left-4 w-px -translate-x-1/2 bg-border"
        />
      )}
      <span
        className={cn(
          'relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border [&_svg]:size-4',
          t.soft,
          t.border,
          t.text,
        )}
      >
        {item.icon}
      </span>
      {item.onClick ? (
        <button
          type="button"
          onClick={item.onClick}
          className="-mx-2 -my-1 min-w-0 flex-1 rounded-lg px-2 py-1 text-left outline-none transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/60"
        >
          {content}
        </button>
      ) : (
        <div className="min-w-0 flex-1 py-0.5">{content}</div>
      )}
    </li>
  )
}

export function Timeline({ items, loading, emptyLabel }: TimelineProps) {
  if (loading) return <SkeletonList rows={4} />
  if (items.length === 0) {
    return emptyLabel ? (
      <p className="py-4 text-sm text-muted-foreground">{emptyLabel}</p>
    ) : null
  }
  return (
    <ol className="flex flex-col">
      {items.map((item, i) => (
        <Row key={item.id} item={item} last={i === items.length - 1} />
      ))}
    </ol>
  )
}
