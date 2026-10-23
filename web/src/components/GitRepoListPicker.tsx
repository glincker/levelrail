import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowsClockwiseIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  fromGitRepoItem,
  normalizeRepoUrl,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'
import {
  GitRepoRateLimitError,
  useGitRepoList,
  useRefreshGitRepos,
} from '../queries/gitRepos'
import type { GitRepoListParams, GitRepoProvider } from '../types/gitRepos'
import { GitRepoCard } from './GitRepoCard'
import { GitRepoListStatus } from './GitRepoListStatus'
import { clock } from '../lib/clockTime'
import { gitProviderKeys } from '../queries/gitProviders'
import { sessionQueryOptions } from '../queries/security'
import {
  readAccountFilter,
  resolveAccountFilter,
  writeAccountFilter,
} from '../lib/gitAccounts'
import { BrandIcon } from './BrandIcon'

const SEARCH_DEBOUNCE_MS = 250
const MAX_OWNER_CHIPS = 8

function FilterChip({
  label,
  active,
  onClick,
  icon,
}: Readonly<{
  label: string
  active: boolean
  onClick: () => void
  icon?: React.ReactNode
}>) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
        active
          ? 'border-primary/50 bg-primary/10 text-primary'
          : 'border-border text-muted-foreground hover:bg-muted',
      )}
    >
      {icon}
      {label}
    </button>
  )
}

// One picker for every provider: searched, owner/archived/fork filtered and
// paged on the server, with the list's completeness and freshness shown.
export function GitRepoListPicker({
  provider,
  enabled,
  noun = 'repositories',
  selectedKey,
  onSelect,
  runningRepoByUrl,
  searchInputId,
  disabled,
}: Readonly<{
  provider: GitRepoProvider
  enabled: boolean
  noun?: 'repositories' | 'projects'
  selectedKey: string
  onSelect: (option: NormalizedRepoOption) => void
  runningRepoByUrl: Map<string, string>
  searchInputId?: string
  disabled?: boolean
}>) {
  const { t } = useTranslation('gitRepos')
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  const [owner, setOwner] = useState('')
  const session = useQuery(sessionQueryOptions())
  const user = session.data?.username ?? ''
  const [chosen, setChosen] = useState<string | null>(null)
  const connection = chosen ?? readAccountFilter(user, provider)
  const [hideArchived, setHideArchived] = useState(false)
  const [hideForks, setHideForks] = useState(false)

  useEffect(() => {
    const id = setTimeout(() => setDebounced(search.trim()), SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(id)
  }, [search])

  const params = useMemo<GitRepoListParams>(
    () => ({
      search: debounced,
      owner,
      connection,
      archived: hideArchived ? 'exclude' : 'include',
      forks: hideForks ? 'exclude' : 'include',
    }),
    [debounced, owner, connection, hideArchived, hideForks],
  )
  const list = useGitRepoList(provider, params, enabled)
  const refresh = useRefreshGitRepos(provider)
  const queryClient = useQueryClient()
  const failed = list.isError
  useEffect(() => {
    if (failed) {
      void queryClient.invalidateQueries({ queryKey: gitProviderKeys.all })
    }
  }, [failed, queryClient])

  const pages = useMemo(() => list.data?.pages ?? [], [list.data])
  const first = pages[0]
  const last = pages[pages.length - 1]
  const options = useMemo(
    () =>
      pages.flatMap((p) => p.items.map((i) => fromGitRepoItem(provider, i))),
    [pages, provider],
  )
  const labels = {
    searchPlaceholder: t(`picker.${noun}.searchPlaceholder`),
    empty: t(`picker.${noun}.empty`),
  }

  let body: React.ReactNode
  if (list.isLoading) {
    body = (
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton key={index} className="h-16 w-full" />
        ))}
      </div>
    )
  } else if (list.isError) {
    const err = list.error
    const rateLimited = err instanceof GitRepoRateLimitError
    const time = rateLimited ? clock(err.nextCheckAt) : ''
    let message = err.message
    if (rateLimited) {
      message = time
        ? t('picker.status.rateLimitedEmpty', { time })
        : t('picker.status.rateLimitedNoTime')
    }
    body = (
      <div className="space-y-2">
        <p className="text-sm text-destructive">{message}</p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void list.refetch()}
        >
          {t('picker.retry')}
        </Button>
      </div>
    )
  } else if (options.length === 0) {
    body = (
      <p className="py-4 text-center text-xs text-muted-foreground">
        {debounced
          ? t(`picker.${noun}.noMatch`, { search: debounced })
          : labels.empty}
      </p>
    )
  } else {
    body = (
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {options.map((option) => (
          <GitRepoCard
            key={option.key}
            option={option}
            selected={option.key === selectedKey}
            runningAs={runningRepoByUrl.get(normalizeRepoUrl(option.cloneUrl))}
            onSelect={() => onSelect(option)}
          />
        ))}
      </div>
    )
  }

  const owners = first?.owners ?? []
  const accounts = first?.connections ?? []
  const staleChoice =
    first !== undefined &&
    connection !== '' &&
    accounts.length > 0 &&
    resolveAccountFilter(connection, accounts) === ''
  if (staleChoice) setChosen('')
  useEffect(() => {
    if (chosen === '') writeAccountFilter(user, provider, '')
  }, [chosen, user, provider])

  function chooseAccount(id: string) {
    setChosen(id)
    writeAccountFilter(user, provider, id)
  }
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <MagnifyingGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            id={searchInputId}
            value={search}
            disabled={disabled}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={labels.searchPlaceholder}
            aria-label={labels.searchPlaceholder}
            className="h-8 pl-8 text-sm"
          />
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || refresh.isPending || !enabled}
          title={t('picker.refreshHint')}
          onClick={() => refresh.mutate()}
        >
          <ArrowsClockwiseIcon
            className={cn('size-3.5', refresh.isPending && 'animate-spin')}
            aria-hidden="true"
          />
          {refresh.isPending ? t('picker.refreshing') : t('picker.refresh')}
        </Button>
      </div>
      <div
        className="flex flex-wrap gap-1.5"
        role="group"
        aria-label={t('picker.filters.label')}
      >
        {accounts.length > 1 ? (
          <>
            <FilterChip
              label={t('picker.filters.allAccounts')}
              active={connection === ''}
              onClick={() => chooseAccount('')}
            />
            {accounts.map((a) => (
              <FilterChip
                key={a.connection_id}
                label={`${a.account || a.label} (${a.loaded})`}
                active={connection === a.connection_id}
                icon={<BrandIcon name={a.provider} className="size-3.5" />}
                onClick={() => chooseAccount(a.connection_id)}
              />
            ))}
          </>
        ) : null}
        {owners.length > 1 ? (
          <>
            <FilterChip
              label={t('picker.filters.allOwners')}
              active={owner === ''}
              onClick={() => setOwner('')}
            />
            {owners.slice(0, MAX_OWNER_CHIPS).map((o) => (
              <FilterChip
                key={o.login}
                label={`${o.login} (${o.count})`}
                active={owner === o.login}
                onClick={() => setOwner(o.login)}
              />
            ))}
          </>
        ) : null}
        <FilterChip
          label={t('picker.filters.hideArchived')}
          active={hideArchived}
          onClick={() => setHideArchived((v) => !v)}
        />
        <FilterChip
          label={t('picker.filters.hideForks')}
          active={hideForks}
          onClick={() => setHideForks((v) => !v)}
        />
      </div>
      {body}
      {last && !list.isError ? (
        <GitRepoListStatus page={last} shown={options.length} />
      ) : null}
      {list.hasNextPage ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={list.isFetchingNextPage}
          onClick={() => void list.fetchNextPage()}
        >
          {list.isFetchingNextPage
            ? t('picker.loadingMore')
            : t('picker.loadMore')}
        </Button>
      ) : null}
    </div>
  )
}
