import { lazy, Suspense, useCallback, useMemo, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ApiError } from '@/lib/apiError'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useTokenExpiryPolicy } from '../../hooks/useTokenExpiryPolicy'
import { sessionQueryOptions } from '../../queries/security'
import {
  tokenCountsQueryOptions,
  tokenGroupsQueryOptions,
  tokenPagesQueryOptions,
} from '../../queries/tokenList'
import { TOKEN_GROUP_MIN, TOKEN_PAGE_SIZE } from '../../lib/tokenListConfig'
import {
  EMPTY_SELECTION,
  selectionCount,
  type TokenSelection,
} from '../../lib/tokenSelection'
import { FlatTokenList, TokenGroupSection, type ListShared } from './TokenLists'
import { TokensBulkBar } from './TokensBulkBar'
import { TokensToolbar, type StatusFilter } from './TokensToolbar'
import { PruneTokensDialog } from './PruneTokensDialog'
import type {
  ServedToken,
  TokenFilter,
  TokenGroup,
  TokenKind,
  TokenSort,
} from '../../types/tokenList'

const BulkRevokeDialog = lazy(() =>
  import('./BulkRevokeDialog').then((m) => ({ default: m.BulkRevokeDialog })),
)
const RotateTokenDialog = lazy(() =>
  import('./RotateTokenDialog').then((m) => ({ default: m.RotateTokenDialog })),
)

const ROOT_ACTIVE: TokenFilter = { ability: 'root', status: 'active' }

interface TokensExplorerProps {
  status: StatusFilter
  onStatus: (s: StatusFilter) => void
}

export function TokensExplorer({ status, onStatus }: TokensExplorerProps) {
  const { t } = useTranslation('tokenList')
  const policy = useTokenExpiryPolicy()
  const isRoot = useIsRoot()
  const [q, setQ] = useState('')
  const [kind, setKind] = useState<TokenKind | ''>('')
  const [ability, setAbility] = useState('')
  const [sort, setSort] = useState<TokenSort>('created')
  const [groupBy, setGroupBy] = useState<boolean | null>(null)
  const [selection, setSelection] = useState<TokenSelection>(EMPTY_SELECTION)
  const [revokeTarget, setRevokeTarget] = useState<TokenSelection | null>(null)
  const [rotateTarget, setRotateTarget] = useState<ServedToken | null>(null)
  const [message, setMessage] = useState('')
  const [known] = useState(() => new Map<string, ServedToken>())

  const session = useQuery({ ...sessionQueryOptions(), retry: false })
  const tokenSession =
    session.error instanceof ApiError &&
    (session.error.status === 401 || session.error.status === 403)

  const base = useMemo<TokenFilter>(
    () => ({
      ...(q ? { q } : {}),
      ...(kind ? { kind } : {}),
      ...(ability ? { ability } : {}),
    }),
    [q, kind, ability],
  )
  const filter = useMemo<TokenFilter>(
    () => (status === 'all' ? base : { ...base, status }),
    [base, status],
  )

  const counts = useQuery(tokenCountsQueryOptions(base)).data
  const allGroups = useQuery(tokenGroupsQueryOptions({})).data
  const autoGroup = (allGroups?.groups ?? []).some(
    (g) => g.count >= TOKEN_GROUP_MIN,
  )
  const grouped = groupBy ?? autoGroup

  const flat = useInfiniteQuery({
    ...tokenPagesQueryOptions({ filter, sort }),
    enabled: !grouped,
  })
  const tokens = useMemo(
    () => flat.data?.pages.flatMap((p) => p.items) ?? [],
    [flat.data],
  )
  const flatTotal = flat.data?.pages[0]?.total ?? 0
  const groups = useQuery({
    ...tokenGroupsQueryOptions(filter),
    enabled: grouped,
  }).data
  const rootData = useQuery({
    ...tokenGroupsQueryOptions(ROOT_ACTIVE),
    enabled: grouped,
  }).data
  const rootCounts = useMemo(() => {
    const map = new Map<string, number>()
    for (const g of rootData?.groups ?? []) map.set(g.name, g.count)
    return map
  }, [rootData])

  const clear = useCallback(() => {
    setSelection(EMPTY_SELECTION)
  }, [])
  const onSeen = useCallback(
    (seen: readonly ServedToken[]) => {
      for (const tok of seen) known.set(tok.id, tok)
    },
    [known],
  )
  const changed =
    <V,>(set: (v: V) => void) =>
    (v: V) => {
      set(v)
      clear()
    }

  const count = selectionCount(selection)
  const bulkEnabled = !tokenSession
  const shared: ListShared = {
    selection,
    setSelection,
    onRotate: setRotateTarget,
    onSeen,
    bulkEnabled,
  }
  const allLoadedSelected =
    selection.mode === 'ids' &&
    tokens.length > 0 &&
    tokens.every((tok) => selection.ids.has(tok.id))
  const offerAll =
    !grouped && allLoadedSelected && flatTotal > tokens.length
      ? flatTotal
      : null
  const disabledReason = tokenSession
    ? t('bar.needsSession')
    : status === 'revoked'
      ? t('bar.nothingToRevoke')
      : null

  function selectGroup(group: TokenGroup, on: boolean) {
    setSelection(
      on
        ? {
            mode: 'filter',
            filter: { ...filter, name: group.name },
            total: group.count,
          }
        : EMPTY_SELECTION,
    )
  }
  function revokeGroup(group: TokenGroup) {
    setRevokeTarget({
      mode: 'filter',
      filter: { ...base, name: group.name, status: 'active' },
      total: group.active,
    })
  }

  const empty = grouped
    ? groups !== undefined && groups.groups.length === 0
    : flat.isSuccess && tokens.length === 0

  return (
    <div className="space-y-3">
      <TokensToolbar
        q={q}
        onQ={changed(setQ)}
        status={status}
        onStatus={changed(onStatus)}
        counts={counts?.counts}
        total={counts?.total}
        kind={kind}
        onKind={changed(setKind)}
        ability={ability}
        onAbility={changed(setAbility)}
        sort={sort}
        onSort={setSort}
        grouped={grouped}
        onGrouped={changed(setGroupBy)}
      />
      <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
        <span>
          {!grouped && !empty ? (
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                disabled={!bulkEnabled || tokens.length === 0}
                checked={allLoadedSelected}
                aria-label={t('selectAllPage', { count: tokens.length })}
                onChange={(e) => {
                  setSelection(
                    e.target.checked
                      ? { mode: 'ids', ids: new Set(tokens.map((x) => x.id)) }
                      : EMPTY_SELECTION,
                  )
                }}
                className="size-4 accent-(--glinr-accent-solid)"
              />
              {t('summary', { count: flatTotal })}
            </label>
          ) : (
            t('summary', { count: grouped ? (groups?.total ?? 0) : flatTotal })
          )}
        </span>
        <PruneTokensDialog disabled={!isRoot} />
      </div>

      {empty ? (
        <EmptyState
          kind="no-results"
          icon={<KeyGlassIcon className="size-5" aria-hidden="true" />}
          title={t('empty.title')}
          description={t('empty.body')}
        />
      ) : grouped ? (
        <GroupedList
          key={JSON.stringify(filter)}
          groups={groups?.groups ?? []}
          filter={filter}
          sort={sort}
          rootCounts={rootCounts}
          shared={shared}
          onSelectGroup={selectGroup}
          onRevokeGroup={revokeGroup}
        />
      ) : (
        <FlatTokenList
          tokens={tokens}
          total={flatTotal}
          loading={flat.isFetchingNextPage}
          onMore={() => void flat.fetchNextPage()}
          shared={shared}
        />
      )}

      <div role="status" aria-live="polite" className="sr-only">
        {count > 0 ? t('bar.selected', { count }) : message}
      </div>
      <TokensBulkBar
        count={count}
        filterMode={selection.mode === 'filter'}
        offerAllMatching={offerAll}
        disabledReason={disabledReason}
        onSelectAllMatching={() => {
          setSelection({ mode: 'filter', filter, total: flatTotal })
        }}
        onClear={() => {
          clear()
          setMessage(t('bar.cleared'))
        }}
        onRevoke={() => {
          setRevokeTarget(selection)
        }}
      />

      <Suspense fallback={null}>
        {revokeTarget ? (
          <BulkRevokeDialog
            selection={revokeTarget}
            known={known}
            onClose={(result) => {
              setRevokeTarget(null)
              if (result) {
                clear()
                setMessage(t('revoke.announce', result))
              }
            }}
          />
        ) : null}
        {rotateTarget ? (
          <RotateTokenDialog
            token={rotateTarget}
            grace={policy.grace}
            onClose={() => {
              setRotateTarget(null)
            }}
          />
        ) : null}
      </Suspense>
    </div>
  )
}

interface GroupedListProps {
  groups: readonly TokenGroup[]
  filter: TokenFilter
  sort: TokenSort
  rootCounts: ReadonlyMap<string, number>
  shared: ListShared
  onSelectGroup: (group: TokenGroup, on: boolean) => void
  onRevokeGroup: (group: TokenGroup) => void
}

// Group rows are paged client side so a thousand distinct names never mount at once.
function GroupedList({ groups, rootCounts, ...rest }: GroupedListProps) {
  const { t } = useTranslation('tokenList')
  const [limit, setLimit] = useState(TOKEN_PAGE_SIZE)
  const shown = groups.slice(0, limit)
  return (
    <div>
      {shown.map((group) => (
        <TokenGroupSection
          key={group.name}
          group={group}
          rootActive={rootCounts.get(group.name) ?? 0}
          {...rest}
        />
      ))}
      {groups.length > limit ? (
        <div className="flex justify-center py-3">
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setLimit((n) => n + TOKEN_PAGE_SIZE)
            }}
          >
            {t('loadMore')}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
