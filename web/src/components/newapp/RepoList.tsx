import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  GitBranchIcon,
  LockSimpleIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import type { NormalizedRepoOption } from '../../lib/gitRepoOptions'

const ROW_HEIGHT = 56

function lastPushLabel(iso: string | undefined, locale: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const days = Math.round((then - Date.now()) / 86_400_000)
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  if (Math.abs(days) < 1) return rtf.format(0, 'day')
  if (Math.abs(days) < 60) return rtf.format(days, 'day')
  return rtf.format(Math.round(days / 30), 'month')
}

/**
 * Searchable, virtualized list of every repository the connected provider
 * apps can see. Rows show name, language and last push.
 */
export function RepoList({
  repos,
  loading,
  onPick,
}: {
  repos: NormalizedRepoOption[]
  loading: boolean
  onPick: (repo: NormalizedRepoOption) => void
}) {
  const { t, i18n } = useTranslation('newApp')
  const [query, setQuery] = useState('')
  const scrollRef = useRef<HTMLDivElement>(null)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const sorted = [...repos].sort((a, b) =>
      (b.pushedAt ?? '').localeCompare(a.pushedAt ?? ''),
    )
    if (q === '') return sorted
    return sorted.filter(
      (r) =>
        r.displayPath.toLowerCase().includes(q) ||
        (r.language ?? '').toLowerCase().includes(q),
    )
  }, [repos, query])

  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  if (loading) {
    return (
      <div className="space-y-2" aria-busy="true">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-14 w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <div className="relative">
        <MagnifyingGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          placeholder={t('source.repos.search')}
          aria-label={t('source.repos.search')}
          className="pl-8"
        />
      </div>
      {filtered.length === 0 ? (
        <p className="rounded-md border border-dashed border-border p-4 text-sm text-muted-foreground">
          {t('source.repos.noMatch', { query })}
        </p>
      ) : (
        <div
          ref={scrollRef}
          className="max-h-88 overflow-y-auto rounded-md border border-border"
          role="list"
          aria-label={t('source.repos.listLabel')}
        >
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((item) => {
              const repo = filtered[item.index]
              if (!repo) return null
              const pushed = lastPushLabel(repo.pushedAt, i18n.language)
              return (
                <div
                  key={repo.key}
                  role="listitem"
                  className="absolute left-0 w-full border-b border-border"
                  style={{
                    height: ROW_HEIGHT,
                    transform: `translateY(${String(item.start)}px)`,
                  }}
                >
                  <button
                    type="button"
                    onClick={() => {
                      onPick(repo)
                    }}
                    className="flex h-full w-full items-center justify-between gap-3 px-3 text-left hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                  >
                    <span className="min-w-0">
                      <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
                        <span className="truncate">{repo.displayPath}</span>
                        {repo.private ? (
                          <LockSimpleIcon
                            className="size-3.5 shrink-0 text-muted-foreground"
                            aria-label={t('source.repos.private')}
                          />
                        ) : null}
                      </span>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <GitBranchIcon className="size-3" aria-hidden="true" />
                        <span className="truncate font-mono">
                          {repo.defaultBranch}
                        </span>
                        {pushed ? (
                          <span>
                            {t('source.repos.pushed', { when: pushed })}
                          </span>
                        ) : null}
                      </span>
                    </span>
                    {repo.language ? (
                      <Badge variant="muted">{repo.language}</Badge>
                    ) : null}
                  </button>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
