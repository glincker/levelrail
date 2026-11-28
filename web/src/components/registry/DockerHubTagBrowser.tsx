import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { cn } from '@/lib/utils'
import { DOCKER_HUB_TAG_PAGE_SIZE } from '../../lib/dockerHubHost'
import { useRegistryCredentialTags } from '../../queries/registryCredentials'

// Tags come back in one response, so filtering and paging happen here;
// a page never exceeds the page size, which keeps the list short.
export function DockerHubTagBrowser({
  credentialId,
  repository,
  disabled,
  onPick,
}: {
  credentialId: string
  repository: string
  disabled?: boolean
  onPick: (tag: string) => void
}) {
  const { t } = useTranslation('dockerHubConnection')
  const tags = useRegistryCredentialTags(credentialId, repository, true)
  const [filter, setFilter] = useState('')
  const [page, setPage] = useState(1)
  const [picked, setPicked] = useState('')

  const needle = filter.trim().toLowerCase()
  const matching = (tags.data ?? []).filter((tag) =>
    tag.toLowerCase().includes(needle),
  )
  const pageCount = Math.max(
    1,
    Math.ceil(matching.length / DOCKER_HUB_TAG_PAGE_SIZE),
  )
  const current = Math.min(page, pageCount)
  const from = (current - 1) * DOCKER_HUB_TAG_PAGE_SIZE
  const visible = matching.slice(from, from + DOCKER_HUB_TAG_PAGE_SIZE)
  const filterId = `dockerhub-tag-filter-${credentialId}`

  if (tags.isLoading) {
    return (
      <p role="status" className="text-xs text-muted-foreground">
        {t('tags.loading')}
      </p>
    )
  }
  if (tags.isError) {
    return (
      <div className="space-y-2">
        <p role="alert" className="text-xs text-destructive">
          {t('tags.loadFailed', { message: tags.error.message })}
        </p>
        <Button
          type="button"
          size="sm"
          onClick={() => void tags.refetch()}
          disabled={tags.isFetching}
        >
          {t('tags.retry')}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <label htmlFor={filterId} className="block space-y-1 text-xs">
        <span>{t('tags.filterLabel')}</span>
        <Input
          id={filterId}
          value={filter}
          placeholder={t('tags.filterPlaceholder')}
          disabled={disabled}
          onChange={(e) => {
            setFilter(e.target.value)
            setPage(1)
          }}
        />
      </label>
      {visible.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('tags.empty')}</p>
      ) : (
        <ul
          aria-label={t('tags.listLabel', { repository })}
          className="rounded-md border border-border p-1"
        >
          {visible.map((tag) => (
            <li key={tag}>
              <button
                type="button"
                disabled={disabled}
                aria-label={t('tags.pick', { tag })}
                aria-pressed={picked === tag}
                onClick={() => {
                  setPicked(tag)
                  onPick(tag)
                }}
                className={cn(
                  'w-full rounded-md px-2 py-1 text-left font-mono text-sm hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50',
                  picked === tag && 'bg-accent',
                )}
              >
                {tag}
              </button>
            </li>
          ))}
        </ul>
      )}
      {matching.length > DOCKER_HUB_TAG_PAGE_SIZE ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-muted-foreground">
            {t('tags.showing', {
              from: from + 1,
              to: from + visible.length,
              total: matching.length,
            })}
          </span>
          <Pagination
            page={current}
            pageCount={pageCount}
            onPageChange={setPage}
          />
        </div>
      ) : null}
    </div>
  )
}
