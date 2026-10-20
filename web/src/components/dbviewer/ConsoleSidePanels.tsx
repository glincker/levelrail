import { useTranslation } from 'react-i18next'
import { TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useClearQueryHistory,
  useDeleteSavedQuery,
  useQueryHistory,
  useSavedQueries,
} from '../../queries/databaseViewer'

const PREVIEW_CHARS = 140

function preview(sql: string): string {
  const flat = sql.replace(/\s+/g, ' ').trim()
  return flat.length > PREVIEW_CHARS
    ? `${flat.slice(0, PREVIEW_CHARS)}...`
    : flat
}

export function HistoryPanel({
  databaseName,
  onLoad,
}: {
  databaseName: string
  onLoad: (sql: string) => void
}) {
  const { t } = useTranslation('databases')
  const history = useQueryHistory(databaseName)
  const clear = useClearQueryHistory(databaseName)

  if (history.isLoading) {
    return <Skeleton className="h-24 w-full" />
  }
  const rows = history.data ?? []
  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('viewer.console.history.empty')}
      </p>
    )
  }
  return (
    <div className="space-y-2">
      <div className="flex justify-end">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={clear.isPending}
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
          <TrashIcon className="size-3.5" aria-hidden="true" />
          {t('viewer.console.history.clear')}
        </Button>
      </div>
      <ul className="max-h-72 divide-y divide-border/60 overflow-auto rounded-lg border border-border">
        {rows.map((h) => (
          <li key={h.id}>
            <button
              type="button"
              className="block w-full px-3 py-2 text-left hover:bg-muted"
              title={t('viewer.console.history.load')}
              onClick={() => {
                onLoad(h.sql)
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
    </div>
  )
}

export function SavedPanel({
  databaseName,
  onLoad,
}: {
  databaseName: string
  onLoad: (sql: string) => void
}) {
  const { t } = useTranslation('databases')
  const saved = useSavedQueries(databaseName)
  const remove = useDeleteSavedQuery(databaseName)

  if (saved.isLoading) {
    return <Skeleton className="h-24 w-full" />
  }
  const rows = saved.data ?? []
  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('viewer.console.saved.empty')}
      </p>
    )
  }
  return (
    <ul className="max-h-72 divide-y divide-border/60 overflow-auto rounded-lg border border-border">
      {rows.map((q) => (
        <li key={q.id} className="flex items-center gap-1 pr-2">
          <button
            type="button"
            className="min-w-0 flex-1 px-3 py-2 text-left hover:bg-muted"
            title={t('viewer.console.saved.load')}
            onClick={() => {
              onLoad(q.sql)
            }}
          >
            <span className="block truncate text-sm font-medium">{q.name}</span>
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
            <TrashIcon className="size-4" aria-hidden="true" />
          </Button>
        </li>
      ))}
    </ul>
  )
}
