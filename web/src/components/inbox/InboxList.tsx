import { useEffect, useMemo, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'
import type { InboxEntry } from '../../lib/inboxEntries'
import type { InboxRow } from '../../lib/inboxManage'
import { formatAge } from '../../lib/format'
import { SignInDecision } from './SignInDecision'

const SEVERITY_DOT: Record<InboxEntry['severity'], string> = {
  critical: 'bg-destructive',
  warning: 'bg-amber-500',
  info: 'bg-sky-500',
}

type FlatRow =
  | { type: 'entry'; entry: InboxEntry; nested: boolean }
  | { type: 'group'; key: string; entries: InboxEntry[] }

function flatten(rows: InboxRow[], expanded: ReadonlySet<string>): FlatRow[] {
  const out: FlatRow[] = []
  for (const r of rows) {
    if (r.type === 'entry') {
      out.push({ type: 'entry', entry: r.entry, nested: false })
      continue
    }
    out.push({ type: 'group', key: r.key, entries: r.entries })
    if (expanded.has(r.key)) {
      for (const entry of r.entries) {
        out.push({ type: 'entry', entry, nested: true })
      }
    }
  }
  return out
}

export interface InboxListProps {
  rows: InboxRow[]
  selectedId?: string
  checked: ReadonlySet<string>
  expanded: ReadonlySet<string>
  hasMore: boolean
  onOpen: (e: InboxEntry) => void
  onCheck: (id: string, shift: boolean) => void
  onCheckMany: (ids: string[], on: boolean) => void
  onToggleGroup: (key: string) => void
  onArchiveMany: (entries: InboxEntry[]) => void
  onDeleteMany: (entries: InboxEntry[]) => void
  loadMore: () => void
}

export function InboxList(p: InboxListProps) {
  const { t } = useTranslation('inbox')
  const parentRef = useRef<HTMLDivElement>(null)
  const flat = useMemo(() => flatten(p.rows, p.expanded), [p.rows, p.expanded])
  const virtualizer = useVirtualizer({
    count: flat.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 56,
    overscan: 8,
  })
  const items = virtualizer.getVirtualItems()
  const lastIndex = items.at(-1)?.index ?? -1
  const { hasMore, loadMore } = p
  useEffect(() => {
    if (hasMore && lastIndex >= flat.length - 5) loadMore()
  }, [hasMore, lastIndex, flat.length, loadMore])

  return (
    <div
      ref={parentRef}
      role="list"
      aria-label={t('page.listLabel')}
      className="max-h-[70vh] overflow-auto rounded-lg border"
    >
      <div
        className="relative w-full"
        style={{ height: `${String(virtualizer.getTotalSize())}px` }}
      >
        {items.map((v) => {
          const row = flat[v.index]
          if (!row) return null
          return (
            <div
              key={v.key}
              ref={virtualizer.measureElement}
              data-index={v.index}
              role="listitem"
              className="absolute top-0 left-0 w-full border-b"
              style={{ transform: `translateY(${String(v.start)}px)` }}
            >
              {row.type === 'group' ? (
                <GroupRow row={row} p={p} />
              ) : (
                <EntryRow entry={row.entry} nested={row.nested} p={p} />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function GroupRow({
  row,
  p,
}: {
  row: Extract<FlatRow, { type: 'group' }>
  p: InboxListProps
}) {
  const { t } = useTranslation('inbox')
  const head = row.entries[0]
  const ids = row.entries.map((e) => e.id)
  const open = p.expanded.has(row.key)
  const all = ids.every((id) => p.checked.has(id))
  const canDelete = row.entries.every((e) => e.deletable)
  if (!head) return null
  return (
    <div className="flex items-center gap-2 px-3 py-2.5">
      <Checkbox
        aria-label={t('page.selectGroup', { title: head.title })}
        checked={all}
        onCheckedChange={(on) => {
          p.onCheckMany(ids, on)
        }}
      />
      <button
        type="button"
        aria-expanded={open}
        onClick={() => {
          p.onToggleGroup(row.key)
        }}
        className="flex min-w-0 flex-1 items-center gap-2 text-left text-sm"
      >
        {open ? (
          <ChevronDownGlassIcon size="1.2em" />
        ) : (
          <ChevronRightGlassIcon size="1.2em" />
        )}
        <span className="truncate font-medium">
          {t('page.group.label', { title: head.title, count: ids.length })}
        </span>
        <span className="shrink-0 text-xs text-muted-foreground">
          {head.at ? formatAge(head.at) : ''}
        </span>
      </button>
      <Button
        type="button"
        size="icon-sm"
        variant="ghost"
        aria-label={t('page.group.archive', { count: ids.length })}
        onClick={() => {
          p.onArchiveMany(row.entries)
        }}
      >
        <ArchiveGlassIcon size="1.2em" />
      </Button>
      {canDelete ? (
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t('page.group.delete', { count: ids.length })}
          onClick={() => {
            p.onDeleteMany(row.entries)
          }}
        >
          <TrashGlassIcon size="1.2em" />
        </Button>
      ) : null}
    </div>
  )
}

function EntryRow({
  entry: e,
  nested,
  p,
}: {
  entry: InboxEntry
  nested: boolean
  p: InboxListProps
}) {
  const { t } = useTranslation('inbox')
  return (
    <>
      <div
        className={cn(
          'flex items-start gap-2 px-3 py-2.5 hover:bg-muted',
          nested && 'pl-9',
          e.id === p.selectedId && 'bg-muted',
        )}
      >
        <Checkbox
          className="mt-1"
          aria-label={t('page.selectItem', { title: e.title })}
          checked={p.checked.has(e.id)}
          onCheckedChange={() => undefined}
          onClick={(ev) => {
            p.onCheck(e.id, ev.shiftKey)
          }}
        />
        <button
          type="button"
          aria-current={e.id === p.selectedId}
          onClick={() => {
            p.onOpen(e)
          }}
          className="flex min-w-0 flex-1 items-start gap-3 text-left"
        >
          <span
            aria-hidden="true"
            className={cn(
              'mt-1.5 size-2 shrink-0 rounded-full',
              e.read ? 'bg-muted-foreground/30' : SEVERITY_DOT[e.severity],
            )}
          />
          <span className="min-w-0 flex-1">
            <span
              className={cn(
                'block truncate text-sm',
                e.read ? 'text-muted-foreground' : 'font-medium',
              )}
            >
              {e.title}
            </span>
            <span className="block truncate text-xs text-muted-foreground">
              {t(`page.kind.${e.kind}`)}
              {e.at ? ` - ${formatAge(e.at)}` : ''}
            </span>
          </span>
        </button>
      </div>
      {e.action?.type === 'signIn' && !e.archived ? (
        <div className="px-3 pb-2.5 pl-12">
          <SignInDecision id={e.action.approvalId} />
        </div>
      ) : null}
    </>
  )
}
