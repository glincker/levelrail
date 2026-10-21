import { useCallback, useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArchiveIcon,
  CheckIcon,
  TrayIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import {
  INBOX_KINDS,
  filterInboxEntries,
  type InboxEntry,
  type InboxKind,
} from '../../lib/inboxEntries'
import { formatAge } from '../../lib/format'
import { useArchiveInbox, useMarkInboxRead } from '../../queries/inbox'
import { useInboxEntries } from '../../queries/inboxFeed'
import { InboxAction } from './InboxAction'
import { SignInDecision } from './SignInDecision'

const SEVERITY_DOT: Record<InboxEntry['severity'], string> = {
  critical: 'bg-destructive',
  warning: 'bg-amber-500',
  info: 'bg-sky-500',
}

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
  const { entries, isPending } = useInboxEntries(archived)
  const mark = useMarkInboxRead()
  const archive = useArchiveInbox()
  const list = filterInboxEntries(entries, { unreadOnly, kind, archived })
  const selected = list.find((e) => e.id === selectedId) ?? list.at(0)
  const unread = entries.filter((e) => !e.read && !e.archived)

  const select = useCallback(
    (e: InboxEntry) => {
      setSelectedId(e.id)
      if (!e.read) mark.mutate({ ids: [e.id] })
    },
    [mark],
  )
  const toggleArchive = useCallback(
    (e: InboxEntry) => {
      const idx = list.findIndex((x) => x.id === e.id)
      archive.mutate({ ids: [e.id], restore: e.archived })
      setSelectedId((list[idx + 1] ?? list[idx - 1])?.id ?? null)
    },
    [archive, list],
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
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  }, [list, selected, select, toggleArchive])

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
          <CheckIcon />
          {t('markAllRead')}
        </Button>
      </div>

      {isPending ? (
        <ListSkeleton rows={5} />
      ) : list.length === 0 ? (
        <EmptyState
          icon={<TrayIcon className="size-5" />}
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
          <ul
            aria-label={t('page.listLabel')}
            className="divide-y rounded-lg border"
          >
            {list.map((e) => (
              <li key={e.id}>
                <button
                  type="button"
                  aria-current={e.id === selected?.id}
                  onClick={() => {
                    select(e)
                  }}
                  className={cn(
                    'flex w-full items-start gap-3 px-3 py-2.5 text-left hover:bg-muted',
                    e.id === selected?.id && 'bg-muted',
                  )}
                >
                  <span
                    aria-hidden="true"
                    className={cn(
                      'mt-1.5 size-2 shrink-0 rounded-full',
                      e.read
                        ? 'bg-muted-foreground/30'
                        : SEVERITY_DOT[e.severity],
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
                {e.action?.type === 'signIn' && !e.archived ? (
                  <div className="px-3 pb-2.5 pl-8">
                    <SignInDecision id={e.action.approvalId} />
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
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
                  <ArchiveIcon />
                  {t(selected.archived ? 'page.restore' : 'page.archive')}
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">{t('page.keys')}</p>
            </section>
          ) : null}
        </div>
      )}
    </div>
  )
}
