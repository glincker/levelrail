import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  CheckGlassIcon,
  InboxGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { Switch } from '@/components/ui/switch'
import { toast, toastAction } from '@/components/ui/toast'
import {
  INBOX_KINDS,
  filterInboxEntries,
  type InboxEntry,
  type InboxKind,
} from '../../lib/inboxEntries'
import {
  UNDO_SECONDS,
  canDelete,
  deleteWithUndo,
  groupEntries,
  selectedEntries,
  toggleAll,
  toggleSelection,
  type SelectionState,
} from '../../lib/inboxManage'
import { formatAge } from '../../lib/format'
import {
  useArchiveInbox,
  useDeleteInbox,
  useMarkInboxRead,
} from '../../queries/inbox'
import { useInboxEntries } from '../../queries/inboxFeed'
import { InboxAction } from './InboxAction'

const InboxList = lazy(() =>
  import('./InboxList').then((m) => ({ default: m.InboxList })),
)
const InboxBulkBar = lazy(() => import('./InboxBulkBar'))
const InboxTidyMenu = lazy(() => import('./InboxTidyMenu'))

const NO_SELECTION: SelectionState = { selected: new Set(), anchor: null }

function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  )
}

export function InboxView() {
  const { t } = useTranslation('inbox')
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [archived, setArchived] = useState(false)
  const [kind, setKind] = useState<InboxKind | 'all'>('all')
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [pick, setPick] = useState<SelectionState>(NO_SELECTION)
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())
  const lastUndo = useRef<(() => Promise<void>) | null>(null)
  const { entries, isPending, hasMore, loadMore } = useInboxEntries(archived)
  const mark = useMarkInboxRead()
  const archive = useArchiveInbox()
  const del = useDeleteInbox()
  const list = filterInboxEntries(entries, { unreadOnly, kind, archived })
  const rows = useMemo(() => groupEntries(list), [list])
  const listIds = list.map((e) => e.id)
  const selected = list.find((e) => e.id === selectedId) ?? list.at(0)
  const inView = entries.filter((e) => e.archived === archived)
  const unread = entries.filter((e) => !e.read && !e.archived)
  const tabBase = filterInboxEntries(entries, {
    unreadOnly,
    kind: 'all',
    archived,
  })
  const picked = selectedEntries(list, pick.selected)

  const select = useCallback(
    (e: InboxEntry) => {
      setSelectedId(e.id)
      if (!e.read) mark.mutate({ ids: [e.id] })
    },
    [mark],
  )
  const dropFromSelection = useCallback((ids: string[]) => {
    setPick((s) => ({
      anchor: s.anchor,
      selected: new Set([...s.selected].filter((id) => !ids.includes(id))),
    }))
  }, [])
  const archiveMany = useCallback(
    (ents: InboxEntry[]) => {
      const ids = ents.map((e) => e.id)
      archive.mutate({ ids, restore: archived })
      dropFromSelection(ids)
    },
    [archive, archived, dropFromSelection],
  )
  const toggleArchive = useCallback(
    (e: InboxEntry) => {
      const idx = list.findIndex((x) => x.id === e.id)
      archive.mutate({ ids: [e.id], restore: e.archived })
      setSelectedId((list[idx + 1] ?? list[idx - 1])?.id ?? null)
    },
    [archive, list],
  )
  const deleteMany = useCallback(
    (ents: InboxEntry[]) => {
      if (!canDelete(ents)) return
      const ids = ents.map((e) => e.id)
      deleteWithUndo(ids, {
        remove: (x) => del.mutateAsync({ ids: x }),
        restore: (x) => del.mutateAsync({ ids: x, restore: true }),
        notify: (undo) => {
          lastUndo.current = undo
          toast.add({
            title: t('page.deleted', { count: ids.length }),
            timeout: UNDO_SECONDS * 1000,
            actionProps: toastAction(t('page.undo'), () => {
              void undo()
            }),
          })
        },
      })
        .then(() => {
          dropFromSelection(ids)
        })
        .catch(() => {
          toast.add({ title: t('page.deleteError'), type: 'error' })
        })
    },
    [del, dropFromSelection, t],
  )

  useEffect(() => {
    const onKey = (ev: KeyboardEvent) => {
      if (ev.metaKey || ev.ctrlKey || ev.altKey || isTyping(ev.target)) return
      const idx = selected ? list.findIndex((x) => x.id === selected.id) : -1
      if (ev.key === 'j' || ev.key === 'k') {
        const next = list[idx + (ev.key === 'j' ? 1 : -1)]
        if (next) select(next)
        ev.preventDefault()
      } else if (ev.key === 'e' && selected) {
        toggleArchive(selected)
        ev.preventDefault()
      } else if (ev.key === '#' && selected?.deletable) {
        deleteMany([selected])
        ev.preventDefault()
      } else if (ev.key === 'z' && lastUndo.current) {
        void lastUndo.current()
        lastUndo.current = null
        ev.preventDefault()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [list, selected, select, toggleArchive, deleteMany])

  const total = inView.length
  const allChecked =
    list.length > 0 && list.every((e) => pick.selected.has(e.id))

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <div
          role="group"
          aria-label={t('page.kindFilter')}
          className="flex flex-wrap gap-1"
        >
          {(['all', ...INBOX_KINDS] as const).map((k) => (
            <Button
              key={k}
              type="button"
              size="sm"
              variant={kind === k ? 'secondary' : 'ghost'}
              aria-pressed={kind === k}
              onClick={() => {
                setKind(k)
              }}
            >
              {t(`page.kind.${k}`)}
              <span className="text-xs text-muted-foreground">
                {k === 'all'
                  ? tabBase.length
                  : tabBase.filter((e) => e.kind === k).length}
              </span>
            </Button>
          ))}
        </div>
        <label className="flex items-center gap-2 text-sm">
          <Switch
            size="sm"
            checked={unreadOnly}
            onCheckedChange={setUnreadOnly}
          />
          {t('unreadOnly')}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <Switch size="sm" checked={archived} onCheckedChange={setArchived} />
          {t('page.showArchived')}
        </label>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={unread.length === 0 || mark.isPending}
          onClick={() => {
            mark.mutate({ all: true, ids: unread.map((e) => e.id) })
          }}
        >
          <CheckGlassIcon size="1.2em" />
          {t('markAllRead')}
        </Button>
        <Suspense fallback={null}>
          <InboxTidyMenu />
        </Suspense>
      </div>

      <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
        <label className="flex items-center gap-2">
          <Checkbox
            aria-label={t('bulk.selectAll')}
            checked={allChecked}
            disabled={list.length === 0}
            onCheckedChange={() => {
              setPick({
                selected: toggleAll(listIds, pick.selected),
                anchor: null,
              })
            }}
          />
          {t('bulk.selectAll')}
        </label>
        <span aria-live="polite">
          {t(hasMore ? 'page.countMore' : 'page.count', {
            count: total,
            unread: unread.length,
          })}
        </span>
      </div>

      {picked.length > 0 ? (
        <Suspense fallback={null}>
          <InboxBulkBar
            count={picked.length}
            canDelete={canDelete(picked)}
            archivedView={archived}
            busy={archive.isPending || mark.isPending || del.isPending}
            onArchive={() => {
              archiveMany(picked)
            }}
            onMarkRead={() => {
              mark.mutate({ ids: picked.map((e) => e.id) })
            }}
            onDelete={() => {
              deleteMany(picked)
            }}
            onClear={() => {
              setPick(NO_SELECTION)
            }}
          />
        </Suspense>
      ) : null}

      {isPending ? (
        <ListSkeleton rows={5} />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<InboxGlassIcon className="size-5.5" />}
          title={t(archived ? 'page.emptyArchivedTitle' : 'page.emptyTitle')}
          description={t(
            archived ? 'page.emptyArchivedBody' : 'page.emptyBody',
          )}
          action={
            archived ? undefined : (
              <Button
                size="sm"
                variant="outline"
                render={<Link to="/alerts" />}
                nativeButton={false}
              >
                {t('page.emptyAction')}
              </Button>
            )
          }
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <Suspense fallback={<ListSkeleton rows={5} />}>
            <InboxList
              rows={rows}
              selectedId={selected?.id}
              checked={pick.selected}
              expanded={expanded}
              hasMore={hasMore}
              loadMore={loadMore}
              onOpen={select}
              onCheck={(id, shift) => {
                setPick((s) => toggleSelection(s, listIds, id, shift))
              }}
              onCheckMany={(ids, on) => {
                setPick((s) => {
                  const next = new Set(s.selected)
                  for (const id of ids) {
                    if (on) next.add(id)
                    else next.delete(id)
                  }
                  return { selected: next, anchor: s.anchor }
                })
              }}
              onToggleGroup={(key) => {
                setExpanded((s) => {
                  const next = new Set(s)
                  if (!next.delete(key)) next.add(key)
                  return next
                })
              }}
              onArchiveMany={archiveMany}
              onDeleteMany={deleteMany}
            />
          </Suspense>
          {selected ? (
            <section
              aria-label={t('page.detailLabel')}
              className="space-y-3 rounded-lg border p-4"
            >
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">
                  {t(`severity.${selected.severity}`)}
                </Badge>
                <Badge variant="muted">{t(`page.kind.${selected.kind}`)}</Badge>
                {selected.at ? (
                  <span className="text-xs text-muted-foreground">
                    {formatAge(selected.at)}
                  </span>
                ) : null}
              </div>
              <h2 className="text-base font-medium">{selected.title}</h2>
              {selected.detail ? (
                <p className="text-sm break-words text-muted-foreground">
                  {selected.detail}
                </p>
              ) : null}
              <div className="flex flex-wrap items-center gap-2">
                {selected.action && !selected.archived ? (
                  <InboxAction
                    action={selected.action}
                    onDone={() => {
                      archive.mutate({ ids: [selected.id] })
                    }}
                  />
                ) : null}
                <Button
                  size="sm"
                  variant="outline"
                  render={<Link to={selected.href} />}
                  nativeButton={false}
                >
                  {t('page.openSource')}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={archive.isPending}
                  onClick={() => {
                    toggleArchive(selected)
                  }}
                >
                  <ArchiveGlassIcon size="1.2em" />
                  {t(selected.archived ? 'page.restore' : 'page.archive')}
                </Button>
                {selected.deletable ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={del.isPending}
                    onClick={() => {
                      deleteMany([selected])
                    }}
                  >
                    <TrashGlassIcon size="1.2em" />
                    {t('bulk.delete')}
                  </Button>
                ) : null}
              </div>
              <p className="text-xs text-muted-foreground">{t('page.keys')}</p>
            </section>
          ) : null}
        </div>
      )}
    </div>
  )
}
