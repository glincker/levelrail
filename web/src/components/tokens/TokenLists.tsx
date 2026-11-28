import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  EllipsisGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { RelativeTime } from '../kit/RelativeTime'
import { TokenListHeader, TokenRowItem } from './TokenRowItem'
import { tokenPagesQueryOptions } from '../../queries/tokenList'
import {
  TOKEN_GROUP_COLLAPSE,
  TOKEN_GROUP_PAGE_SIZE,
  TOKEN_ROOT_WARN_MIN,
} from '../../lib/tokenListConfig'
import {
  addIds,
  rangeIds,
  removeIds,
  toggleId,
  type TokenSelection,
} from '../../lib/tokenSelection'
import type {
  ServedToken,
  TokenFilter,
  TokenGroup,
  TokenSort,
} from '../../types/tokenList'

export interface ListShared {
  selection: TokenSelection
  setSelection: (next: TokenSelection) => void
  onRotate: (token: ServedToken) => void
  // Lists report what they loaded so the bulk dialog can describe an id selection.
  onSeen: (tokens: readonly ServedToken[]) => void
  bulkEnabled: boolean
}

function useRowSelect(order: readonly string[], shared: ListShared) {
  const anchor = useRef<string | null>(null)
  const { selection, setSelection } = shared
  const selRef = useRef(selection)
  const orderRef = useRef(order)
  useEffect(() => {
    selRef.current = selection
    orderRef.current = order
  })
  return useCallback(
    (id: string, shift: boolean) => {
      const from = anchor.current
      if (shift && from && from !== id) {
        const ids = rangeIds(orderRef.current, from, id)
        const turnOn = !(
          selRef.current.mode === 'ids' && selRef.current.ids.has(id)
        )
        setSelection(
          turnOn ? addIds(selRef.current, ids) : removeIds(selRef.current, ids),
        )
      } else {
        setSelection(toggleId(selRef.current, id))
      }
      anchor.current = id
    },
    [setSelection],
  )
}

function Rows({
  tokens,
  shared,
}: {
  tokens: readonly ServedToken[]
  shared: ListShared
}) {
  const order = useMemo(() => tokens.map((tok) => tok.id), [tokens])
  const onSelect = useRowSelect(order, shared)
  const { selection, onSeen, onRotate } = shared
  useEffect(() => {
    onSeen(tokens)
  }, [tokens, onSeen])
  return (
    <ul>
      {tokens.map((token) => (
        <TokenRowItem
          key={token.id}
          token={token}
          selected={selection.mode === 'ids' && selection.ids.has(token.id)}
          onSelect={onSelect}
          onRotate={onRotate}
        />
      ))}
    </ul>
  )
}

function LoadMoreRow({
  loaded,
  total,
  loading,
  onClick,
}: {
  loaded: number
  total: number
  loading: boolean
  onClick: () => void
}) {
  const { t } = useTranslation('tokenList')
  return (
    <div className="flex items-center justify-center gap-3 py-3">
      <span className="text-xs text-muted-foreground">
        {t('loaded', { loaded, total })}
      </span>
      {loaded < total ? (
        <Button
          size="sm"
          variant="outline"
          disabled={loading}
          onClick={onClick}
        >
          {t('loadMore')}
        </Button>
      ) : null}
    </div>
  )
}

interface FlatListProps {
  tokens: readonly ServedToken[]
  total: number
  loading: boolean
  onMore: () => void
  shared: ListShared
}

export function FlatTokenList({
  tokens,
  total,
  loading,
  onMore,
  shared,
}: FlatListProps) {
  return (
    <div>
      <TokenListHeader />
      <Rows tokens={tokens} shared={shared} />
      <LoadMoreRow
        loaded={tokens.length}
        total={total}
        loading={loading}
        onClick={onMore}
      />
    </div>
  )
}

interface GroupSectionProps {
  group: TokenGroup
  filter: TokenFilter
  sort: TokenSort
  rootActive: number
  shared: ListShared
  onSelectGroup: (group: TokenGroup, on: boolean) => void
  onRevokeGroup: (group: TokenGroup) => void
}

export function TokenGroupSection({
  group,
  filter,
  sort,
  rootActive,
  shared,
  onSelectGroup,
  onRevokeGroup,
}: GroupSectionProps) {
  const { t } = useTranslation('tokenList')
  const [open, setOpen] = useState(group.count <= TOKEN_GROUP_COLLAPSE)
  const bodyId = useId()
  const { selection, bulkEnabled } = shared
  const groupSelected =
    selection.mode === 'filter' && selection.filter.name === group.name
  const warn = rootActive >= TOKEN_ROOT_WARN_MIN
  return (
    <section
      aria-label={group.name}
      className={cn(
        'border-b border-border',
        warn && 'bg-tone-warning-soft/40',
      )}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5">
        <input
          type="checkbox"
          checked={groupSelected}
          disabled={!bulkEnabled}
          aria-label={t('group.selectGroup', { name: group.name })}
          onChange={(e) => {
            onSelectGroup(group, e.target.checked)
          }}
          className="size-4 shrink-0 accent-(--glinr-accent-solid)"
        />
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          aria-label={t('group.toggle', {
            name: group.name,
            count: group.count,
          })}
          onClick={() => {
            setOpen((v) => !v)
          }}
          className="flex min-w-0 flex-1 items-center gap-2 rounded-(--glinr-radius-sm) text-left text-sm font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <ChevronDownGlassIcon
            aria-hidden="true"
            className={cn(
              'size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none',
              !open && '-rotate-90',
            )}
          />
          <span className="truncate">{group.name}</span>
          <span className="shrink-0 text-xs font-normal text-muted-foreground">
            {t('group.count', { count: group.count })}
          </span>
        </button>
        <span className="flex flex-wrap items-center gap-x-3 text-xs text-muted-foreground">
          <span>{t('group.active', { count: group.active })}</span>
          {group.expiring > 0 ? (
            <span>{t('group.expiring', { count: group.expiring })}</span>
          ) : null}
          {group.expired > 0 ? (
            <span>{t('group.expired', { count: group.expired })}</span>
          ) : null}
          <span>
            {t('group.oldest')} <RelativeTime at={group.oldest_created} />
          </span>
          <span>
            {group.newest_last_used ? (
              <>
                {t('group.lastUsed')}{' '}
                <RelativeTime at={group.newest_last_used} />
              </>
            ) : (
              t('group.neverUsed')
            )}
          </span>
        </span>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={!bulkEnabled || group.active === 0}
                aria-label={t('group.menu', { name: group.name })}
              />
            }
          >
            <EllipsisGlassIcon aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem
              onClick={() => {
                onRevokeGroup(group)
              }}
            >
              {t('group.revokeActive')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      {warn ? (
        <p
          role="note"
          className="flex items-center gap-1.5 px-3 pb-2 text-xs text-tone-warning"
        >
          <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
          {t('group.rootWarning', { count: rootActive, name: group.name })}
        </p>
      ) : null}
      {open ? (
        <div id={bodyId}>
          <GroupTokens
            name={group.name}
            filter={filter}
            sort={sort}
            shared={shared}
          />
        </div>
      ) : null}
    </section>
  )
}

function GroupTokens({
  name,
  filter,
  sort,
  shared,
}: {
  name: string
  filter: TokenFilter
  sort: TokenSort
  shared: ListShared
}) {
  const query = useInfiniteQuery(
    tokenPagesQueryOptions({
      filter: { ...filter, name },
      sort,
      limit: TOKEN_GROUP_PAGE_SIZE,
    }),
  )
  const tokens = useMemo(
    () => query.data?.pages.flatMap((p) => p.items) ?? [],
    [query.data],
  )
  const total = query.data?.pages[0]?.total ?? 0
  return (
    <div className="border-t border-border/60 bg-background/40">
      <Rows tokens={tokens} shared={shared} />
      {tokens.length < total ? (
        <LoadMoreRow
          loaded={tokens.length}
          total={total}
          loading={query.isFetchingNextPage}
          onClick={() => void query.fetchNextPage()}
        />
      ) : null}
    </div>
  )
}
