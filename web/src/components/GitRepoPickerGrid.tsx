import { useMemo, useState } from 'react'
import {
  CheckCircleIcon,
  GitBranchIcon,
  LockSimpleIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  normalizeRepoUrl,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'

function GitRepoCard({
  option,
  selected,
  runningAs,
  onSelect,
}: {
  option: NormalizedRepoOption
  selected: boolean
  runningAs?: string
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onSelect}
      className={cn(
        'flex flex-col items-start gap-1.5 rounded-lg border border-border bg-card p-2.5 text-left transition-colors hover:border-primary/40 hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        selected && 'border-primary/50 bg-muted',
      )}
    >
      <span className="flex w-full items-center gap-1.5 text-sm font-medium text-foreground">
        <span className="truncate">{option.displayPath}</span>
        {option.private ? (
          <LockSimpleIcon
            className="size-3.5 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
        ) : null}
      </span>
      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <GitBranchIcon className="size-3 shrink-0" aria-hidden="true" />
        <span className="truncate font-mono">{option.defaultBranch}</span>
      </span>
      {runningAs ? (
        <span className="flex items-center gap-1 text-xs font-medium text-primary">
          <CheckCircleIcon className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">Running as {runningAs}</span>
        </span>
      ) : null}
    </button>
  )
}

// Capped rather than virtualized: search already narrows fast, and a
// responsive grid needs column-aware virtualization to window correctly.
const GRID_RESULT_CAP = 100

export function RepoPickerGrid({
  options,
  isLoading,
  isError,
  errorMessage,
  selectedKey,
  onSelect,
  runningRepoByUrl,
  searchInputId,
  searchPlaceholder = 'Search repositories...',
  emptyMessage = 'No repositories found.',
  disabled,
}: {
  options: NormalizedRepoOption[]
  isLoading: boolean
  isError: boolean
  errorMessage?: string
  selectedKey: string
  onSelect: (option: NormalizedRepoOption) => void
  runningRepoByUrl: Map<string, string>
  searchInputId?: string
  searchPlaceholder?: string
  emptyMessage?: string
  disabled?: boolean
}) {
  const [search, setSearch] = useState('')

  const sorted = useMemo(() => {
    return [...options].sort((a, b) => {
      const aRunning = runningRepoByUrl.has(normalizeRepoUrl(a.cloneUrl))
      const bRunning = runningRepoByUrl.has(normalizeRepoUrl(b.cloneUrl))
      if (aRunning !== bRunning) return aRunning ? -1 : 1
      return a.displayPath.localeCompare(b.displayPath)
    })
  }, [options, runningRepoByUrl])

  const normalizedSearch = search.trim().toLowerCase()
  const filtered = normalizedSearch
    ? sorted.filter((option) =>
        option.displayPath.toLowerCase().includes(normalizedSearch),
      )
    : sorted
  const visible = filtered.slice(0, GRID_RESULT_CAP)

  return (
    <div className="space-y-2">
      <div className="relative">
        <MagnifyingGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          id={searchInputId}
          value={search}
          disabled={disabled}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={searchPlaceholder}
          aria-label={searchPlaceholder}
          className="h-8 pl-8 text-sm"
        />
      </div>
      {isLoading ? (
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      ) : isError ? (
        <p className="text-sm text-destructive">{errorMessage}</p>
      ) : visible.length === 0 ? (
        <p className="py-4 text-center text-xs text-muted-foreground">
          {search ? `No repositories match "${search}".` : emptyMessage}
        </p>
      ) : (
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {visible.map((option) => (
            <GitRepoCard
              key={option.key}
              option={option}
              selected={option.key === selectedKey}
              runningAs={runningRepoByUrl.get(
                normalizeRepoUrl(option.cloneUrl),
              )}
              onSelect={() => onSelect(option)}
            />
          ))}
        </div>
      )}
      {filtered.length > GRID_RESULT_CAP ? (
        <p className="text-xs text-muted-foreground">
          Showing the first {GRID_RESULT_CAP} of {filtered.length} matches.
          Refine your search to narrow the list.
        </p>
      ) : null}
    </div>
  )
}
