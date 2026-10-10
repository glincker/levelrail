import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { useRedisKey, useRedisKeys } from '../../queries/databaseViewer'
import type { RedisKey } from '../../types/databaseViewer'

// Read-only key browser for Redis-compatible engines. Pages through SCAN
// server-side; nothing here can write.
export function RedisKeyBrowser({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databases')
  const [patternInput, setPatternInput] = useState('*')
  const [pattern, setPattern] = useState('*')
  const [selected, setSelected] = useState<string | null>(null)

  const scan = useRedisKeys(databaseName, pattern)
  const value = useRedisKey(databaseName, selected)

  const collected = useMemo(() => {
    const seen = new Set<string>()
    const out: RedisKey[] = []
    for (const page of scan.data?.pages ?? []) {
      for (const k of page.keys) {
        if (!seen.has(k.key)) {
          seen.add(k.key)
          out.push(k)
        }
      }
    }
    return out
  }, [scan.data])
  const done = !scan.hasNextPage

  function submit() {
    setSelected(null)
    setPattern(patternInput.trim() || '*')
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        {t('viewer.keys.readOnlyNote')}
      </p>
      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <label className="flex-1 space-y-1 text-xs text-muted-foreground">
          <span>{t('viewer.keys.patternLabel')}</span>
          <Input
            value={patternInput}
            placeholder={t('viewer.keys.patternPlaceholder')}
            onChange={(e) => {
              setPatternInput(e.target.value)
            }}
          />
        </label>
        <Button type="submit" size="sm">
          <MagnifyingGlassIcon className="size-3.5" aria-hidden="true" />
          {t('viewer.keys.scan')}
        </Button>
      </form>

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="space-y-2">
          {scan.isLoading ? (
            <Skeleton className="h-48 w-full" />
          ) : scan.error ? (
            <p className="text-sm text-destructive" role="alert">
              {scan.error.message}
            </p>
          ) : collected.length === 0 && done ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              {t('viewer.keys.empty')}
            </p>
          ) : (
            <ul className="max-h-[28rem] divide-y divide-border/60 overflow-auto rounded-lg border border-border">
              {collected.map((k) => (
                <li key={k.key}>
                  <button
                    type="button"
                    aria-current={selected === k.key ? 'true' : undefined}
                    className={cn(
                      'flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left hover:bg-muted',
                      selected === k.key && 'bg-muted',
                    )}
                    onClick={() => {
                      setSelected(k.key)
                    }}
                  >
                    <span className="truncate font-mono text-xs">{k.key}</span>
                    <Badge variant="muted">{k.type}</Badge>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span>
              {done && collected.length > 0
                ? t('viewer.keys.scanComplete')
                : ''}
            </span>
            {done ? null : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={scan.isFetching}
                onClick={() => {
                  void scan.fetchNextPage()
                }}
              >
                {t('viewer.keys.loadMore')}
              </Button>
            )}
          </div>
        </div>

        <div className="min-w-0 rounded-lg border border-border p-3">
          {selected === null ? (
            <p className="text-sm text-muted-foreground">
              {t('viewer.keys.selectKey')}
            </p>
          ) : value.isLoading ? (
            <Skeleton className="h-24 w-full" />
          ) : value.error ? (
            <p className="text-sm text-destructive" role="alert">
              {value.error.message}
            </p>
          ) : value.data ? (
            <div className="space-y-2">
              <p className="break-all font-mono text-sm font-medium">
                {selected}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('viewer.keys.type')}: {value.data.type} |{' '}
                {t('viewer.keys.ttl')}:{' '}
                {value.data.ttl < 0
                  ? t('viewer.keys.noTtl')
                  : t('viewer.keys.ttlSeconds', { count: value.data.ttl })}
              </p>
              <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-3 font-mono text-xs">
                {value.data.lines.join('\n')}
              </pre>
              {value.data.truncated ? (
                <p className="text-xs text-muted-foreground">
                  {t('viewer.keys.truncated')}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}
