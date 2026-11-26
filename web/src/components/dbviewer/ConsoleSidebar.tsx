import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useClearQueryHistory,
  useDeleteSavedQuery,
  useQueryHistory,
  useSavedQueries,
} from '../../queries/databaseViewer'

const PREVIEW_CHARS = 100

function preview(sql: string): string {
  const flat = sql.replace(/\s+/g, ' ').trim()
  return flat.length > PREVIEW_CHARS
    ? `${flat.slice(0, PREVIEW_CHARS)}...`
    : flat
}

// Saved statements and recent history with one shared search box. A click
// opens the statement in a new editor tab.
export function ConsoleSidebar({
  databaseName,
  onOpen,
}: {
  databaseName: string
  onOpen: (sql: string, title: string | null) => void
}) {
  const { t } = useTranslation('databases')
  const [query, setQuery] = useState('')
  const saved = useSavedQueries(databaseName)
  const history = useQueryHistory(databaseName)
  const remove = useDeleteSavedQuery(databaseName)
  const clear = useClearQueryHistory(databaseName)
  const needle = query.trim().toLowerCase()
  const match = (...parts: string[]) =>
    needle === '' || parts.some((p) => p.toLowerCase().includes(needle))
  const savedRows = (saved.data ?? []).filter((q) => match(q.name, q.sql))
  const historyRows = (history.data ?? []).filter((h) => match(h.sql))

  return (
    <aside className="space-y-3" aria-label={t('viewer.console.sidebar.label')}>
      <div className="relative">
        <SearchGlassIcon
          className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          type="search"
          value={query}
          className="pl-8"
          aria-label={t('viewer.console.sidebar.searchLabel')}
          placeholder={t('viewer.console.sidebar.searchPlaceholder')}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
        />
      </div>

      <section className="space-y-1">
        <h2 className="text-sm font-medium">{t('viewer.console.tabSaved')}</h2>
        {saved.isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : savedRows.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {needle === ''
              ? t('viewer.console.saved.empty')
              : t('viewer.console.sidebar.noMatch')}
          </p>
        ) : (
          <ul className="max-h-56 divide-y divide-border/60 overflow-auto rounded-lg border border-border">
            {savedRows.map((q) => (
              <li key={q.id} className="flex items-center pr-1">
                <button
                  type="button"
                  className="min-w-0 flex-1 px-2 py-1.5 text-left hover:bg-muted"
                  title={t('viewer.console.sidebar.openInTab')}
                  onClick={() => {
                    onOpen(q.sql, q.name)
                  }}
                >
                  <span className="block truncate text-sm font-medium">
                    {q.name}
                  </span>
                  <span className="block truncate font-mono text-xs text-muted-foreground">
                    {preview(q.sql)}
                  </span>
                </button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  disabled={remove.isPending}
                  title={t('viewer.console.saved.delete')}
                  aria-label={t('viewer.console.saved.delete')}
                  onClick={() => {
                    remove.mutate(q.id)
                  }}
                >
                  <TrashGlassIcon className="size-5" aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-1">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-medium">
            {t('viewer.console.tabHistory')}
          </h2>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={clear.isPending || (history.data ?? []).length === 0}
            onClick={() => {
              clear.mutate(undefined, {
                onSuccess: () => {
                  toast.add({
                    title: t('viewer.console.history.cleared'),
                    type: 'success',
                  })
                },
              })
            }}
          >
            {t('viewer.console.history.clear')}
          </Button>
        </div>
        {history.isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : historyRows.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {needle === ''
              ? t('viewer.console.history.empty')
              : t('viewer.console.sidebar.noMatch')}
          </p>
        ) : (
          <ul className="max-h-72 divide-y divide-border/60 overflow-auto rounded-lg border border-border">
            {historyRows.map((h) => (
              <li key={h.id}>
                <button
                  type="button"
                  className="block w-full px-2 py-1.5 text-left hover:bg-muted"
                  title={t('viewer.console.sidebar.openInTab')}
                  onClick={() => {
                    onOpen(h.sql, null)
                  }}
                >
                  <span className="block truncate font-mono text-xs">
                    {preview(h.sql)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {new Date(h.created_at).toLocaleString()} |{' '}
                    {t('viewer.console.elapsed', { ms: h.duration_ms })}
                    {h.ok ? '' : ` | ${t('viewer.console.history.failed')}`}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  )
}
