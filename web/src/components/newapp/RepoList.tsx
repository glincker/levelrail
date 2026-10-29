import { useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  GitBranchGlassIcon,
  LinkGlassIcon,
  LockGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { BrandTile } from '../BrandLogoBadge'
import type { NormalizedRepoOption } from '../../lib/gitRepoOptions'
import { filterRepos } from '../../lib/repoFilter'
import { hostBrand } from '../../lib/repoImport'

const ROW_HEIGHT = 48

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

function hostOf(url: string): string {
  const m = /^(?:[a-z+]+:\/\/(?:[^@/]+@)?|[\w.-]+@)([^/:]+)/i.exec(url)
  return m?.[1] ?? ''
}

/**
 * Virtualized rows for the one repository field: a detected URL comes first
 * as "Use this URL", then the connected repositories that match the text.
 */
export function RepoList({
  repos,
  loading,
  query,
  urlCandidate,
  hasError,
  refreshing,
  onPick,
  onUseUrl,
  onRefresh,
}: {
  repos: NormalizedRepoOption[]
  loading: boolean
  query: string
  urlCandidate: string | null
  hasError: boolean
  refreshing: boolean
  onPick: (repo: NormalizedRepoOption) => void
  onUseUrl: (url: string) => void
  onRefresh: () => void
}) {
  const { t, i18n } = useTranslation('newApp')
  const scrollRef = useRef<HTMLDivElement>(null)
  const filtered = useMemo(
    () => filterRepos(repos, query, urlCandidate),
    [repos, query, urlCandidate],
  )
  const virtualizer = useVirtualizer({
    count: filtered.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  if (loading && !urlCandidate) {
    return (
      <div
        className="space-y-1.5"
        role="status"
        aria-busy="true"
        aria-label={t('source.repos.loading')}
      >
        <Skeleton className="h-11 w-full" />
        <Skeleton className="h-11 w-full" />
        <Skeleton className="h-11 w-full" />
      </div>
    )
  }

  if (repos.length === 0 && !urlCandidate) {
    return (
      <div className="space-y-2 rounded-lg border border-dashed border-border p-4">
        <p className="text-sm font-medium text-foreground">
          {hasError
            ? t('source.repos.failedTitle')
            : t('source.repos.emptyTitle')}
        </p>
        <p className="text-sm text-muted-foreground">
          {hasError
            ? t('source.repos.failedBody')
            : t('source.repos.emptyBody')}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={refreshing}
            onClick={onRefresh}
          >
            <RefreshCwGlassIcon size="1.2em" aria-hidden="true" />
            {refreshing
              ? t('source.repos.refreshing')
              : t('source.repos.refresh')}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            render={<Link to="/settings/git-connections" />}
            nativeButton={false}
          >
            {t('source.git.manage')}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      {urlCandidate ? (
        <button
          type="button"
          onClick={() => {
            onUseUrl(urlCandidate)
          }}
          className="flex w-full items-center gap-3 rounded-lg border border-primary/40 bg-muted/40 px-3 py-2 text-left hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
        >
          <BrandTile
            name={hostBrand(hostOf(urlCandidate))}
            label={hostOf(urlCandidate)}
            size="sm"
          />
          <span className="min-w-0 flex-1">
            <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
              <LinkGlassIcon className="size-4" aria-hidden="true" />
              {t('source.find.useUrl')}
            </span>
            <span className="block truncate font-mono text-xs text-muted-foreground">
              {urlCandidate}
            </span>
          </span>
        </button>
      ) : null}
      {filtered.length === 0 && !urlCandidate ? (
        <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
          {t('source.repos.noMatch', { query: query.trim() })}
        </p>
      ) : null}
      {filtered.length > 0 ? (
        <div
          ref={scrollRef}
          className="max-h-72 overflow-y-auto rounded-lg border border-border xl:max-h-96"
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
                          <LockGlassIcon
                            className="size-4 shrink-0 text-muted-foreground"
                            aria-label={t('source.repos.private')}
                          />
                        ) : null}
                      </span>
                      <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                        <GitBranchGlassIcon
                          className="size-3.5"
                          aria-hidden="true"
                        />
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
      ) : null}
      {hasError ? (
        <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          {t('source.repos.partial')}
          <Button
            type="button"
            size="xs"
            variant="ghost"
            disabled={refreshing}
            onClick={onRefresh}
          >
            {t('source.repos.retry')}
          </Button>
        </p>
      ) : null}
    </div>
  )
}
