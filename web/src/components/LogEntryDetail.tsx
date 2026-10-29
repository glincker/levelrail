import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from './ui/button'
import { useLogSearch } from '../queries/logs'
import type { LogEntry } from '../types/logs'

const CONTEXT_WINDOW_MS = 30_000
const CONTEXT_LINES = 5

function flatten(
  value: unknown,
  prefix: string,
  out: { path: string; value: string }[],
): void {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
      flatten(v, prefix ? `${prefix}.${k}` : k, out)
    }
    return
  }
  out.push({
    path: prefix,
    value: typeof value === 'string' ? value : JSON.stringify(value),
  })
}

// One selected line: pretty JSON with a chip per field that adds a filter on
// click, plus the lines logged just before and after it.
export function LogEntryDetail({
  appName,
  entry,
  onAddFilter,
  onClose,
}: {
  appName: string
  entry: LogEntry
  onAddFilter: (path: string, value: string) => void
  onClose: () => void
}) {
  const { t } = useTranslation('observability')
  const at = new Date(entry.timestamp)
  const ctx = useLogSearch(appName, {
    from: new Date(at.getTime() - CONTEXT_WINDOW_MS),
    to: new Date(at.getTime() + CONTEXT_WINDOW_MS),
    container: entry.container,
    limit: 400,
  })
  const { before, after } = useMemo(() => {
    const all = ctx.data?.entries ?? []
    const i = all.findIndex(
      (e) => e.timestamp === entry.timestamp && e.message === entry.message,
    )
    if (i < 0) {
      return { before: [] as LogEntry[], after: [] as LogEntry[] }
    }
    return {
      before: all.slice(Math.max(0, i - CONTEXT_LINES), i),
      after: all.slice(i + 1, i + 1 + CONTEXT_LINES),
    }
  }, [ctx.data, entry])

  const chips = useMemo(() => {
    const out: { path: string; value: string }[] = []
    if (entry.structured && entry.fields) {
      flatten(entry.fields, '', out)
    }
    return out
  }, [entry])

  const line = (e: LogEntry, key: string, emphasize = false) => (
    <p
      key={key}
      className={`font-mono text-xs break-words whitespace-pre-wrap ${
        emphasize ? 'text-foreground' : 'text-muted-foreground'
      }`}
    >
      <span className="mr-2 tabular-nums">
        {new Date(e.timestamp).toLocaleTimeString()}
      </span>
      {e.message}
    </p>
  )

  return (
    <aside
      className="mt-3 rounded-lg border border-border p-3"
      aria-label={t('explorer.entryDetail')}
    >
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('explorer.entryDetail')}
        </h3>
        <div className="flex gap-1">
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t('explorer.copyLine')}
            onClick={() => {
              void navigator.clipboard.writeText(entry.message)
            }}
          >
            <CopyGlassIcon className="size-5" aria-hidden="true" />
          </Button>
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t('explorer.closeDetail')}
            onClick={onClose}
          >
            <XGlassIcon className="size-5" aria-hidden="true" />
          </Button>
        </div>
      </div>

      {chips.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {chips.map((c) => (
            <button
              key={c.path}
              type="button"
              className="max-w-full truncate rounded border border-border px-1.5 py-0.5 font-mono text-xs hover:bg-muted"
              title={t('explorer.filterByField', { field: c.path })}
              onClick={() => {
                onAddFilter(c.path, c.value)
              }}
            >
              <span className="text-muted-foreground">{c.path}=</span>
              {c.value}
            </button>
          ))}
        </div>
      ) : null}

      <pre className="mt-2 max-h-48 overflow-auto rounded bg-neutral-950 p-2 font-mono text-xs break-words whitespace-pre-wrap text-neutral-200">
        {entry.structured && entry.fields
          ? JSON.stringify(entry.fields, null, 2)
          : entry.message}
      </pre>

      <h4 className="mt-3 text-xs font-medium text-muted-foreground">
        {t('explorer.context')}
      </h4>
      <div className="mt-1 space-y-0.5">
        {ctx.isLoading ? (
          <p className="text-xs text-muted-foreground">
            {t('explorer.loading')}
          </p>
        ) : (
          <>
            {before.map((e, i) => line(e, `b${i}`))}
            {line(entry, 'cur', true)}
            {after.map((e, i) => line(e, `a${i}`))}
          </>
        )}
      </div>
    </aside>
  )
}
