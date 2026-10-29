import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, LockGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { useDebouncedValue } from '../../../hooks/useDebouncedValue'
import { useGitRepoList } from '../../../queries/gitRepos'
import type { GitRepoItem } from '../../../types/gitRepos'
import { FieldHelp } from './FieldMessage'
import { sortRecentFirst } from './logic'

const SEARCH_DEBOUNCE_MS = 250
const PICKER_LIMIT = 30

export function RepoPicker({
  selectedUrl,
  onPick,
}: Readonly<{
  selectedUrl: string
  onPick: (repo: GitRepoItem) => void
}>) {
  const { t } = useTranslation('functions')
  const [search, setSearch] = useState('')
  const debounced = useDebouncedValue(search.trim(), SEARCH_DEBOUNCE_MS)
  const query = useGitRepoList(
    '',
    { search: debounced, limit: PICKER_LIMIT, archived: 'exclude' },
    true,
  )
  const items = useMemo(
    () => sortRecentFirst(query.data?.pages.flatMap((p) => p.items) ?? []),
    [query.data],
  )
  const empty = !query.isPending && !query.error && items.length === 0

  return (
    <div className="min-w-0 space-y-2">
      <Input
        aria-label={t('deploy.source.search')}
        className="w-full"
        placeholder={t('deploy.source.search')}
        value={search}
        onChange={(e) => {
          setSearch(e.target.value)
        }}
      />
      <FieldHelp>{t('deploy.source.recent')}</FieldHelp>
      {query.isPending ? (
        <FieldHelp>{t('deploy.source.loading')}</FieldHelp>
      ) : null}
      {query.error ? (
        <p role="alert" className="text-xs text-destructive">
          {t('deploy.source.reposError', { message: query.error.message })}
        </p>
      ) : null}
      {empty ? <FieldHelp>{t('deploy.source.noRepos')}</FieldHelp> : null}
      <ul className="min-w-0 max-h-48 divide-y divide-input overflow-y-auto rounded-lg border border-input empty:hidden">
        {items.map((repo) => {
          const selected = repo.clone_url === selectedUrl
          return (
            <li key={`${repo.connection_id ?? ''}:${repo.key}`}>
              <button
                type="button"
                aria-pressed={selected}
                onClick={() => {
                  onPick(repo)
                }}
                className={cn(
                  'flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-muted focus-visible:bg-muted focus-visible:outline-none',
                  selected && 'bg-muted',
                )}
              >
                <span className="truncate">{repo.full_name}</span>
                <span className="flex shrink-0 items-center gap-1.5">
                  {repo.private ? (
                    <Badge variant="outline">
                      <LockGlassIcon size="1.2em" aria-hidden="true" />
                      {t('deploy.source.private')}
                    </Badge>
                  ) : null}
                  {selected ? (
                    <CheckGlassIcon size="1.2em" aria-hidden="true" />
                  ) : null}
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
