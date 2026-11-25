import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { RedisInfoPanel } from './RedisInfoPanel'
import { RedisValuePanel } from './RedisValuePanel'
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
  const [tab, setTab] = useState<'keys' | 'info'>('keys')
  const [typeFilter, setTypeFilter] = useState('')

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
  const types = useMemo(
    () => Array.from(new Set(collected.map((k) => k.type))).sort(),
    [collected],
  )
  const visible = useMemo(
    () =>
      typeFilter ? collected.filter((k) => k.type === typeFilter) : collected,
    [collected, typeFilter],
  )
  const done = !scan.hasNextPage

  function submit() {
    setSelected(null)
    setPattern(patternInput.trim() || '*')
  }

  const tabs = (
    <div
      className="flex gap-1"
      role="group"
      aria-label={t('viewer.keys.title')}
    >
      {(['keys', 'info'] as const).map((id) => (
        <Button
          key={id}
          type="button"
          size="sm"
          variant={tab === id ? 'default' : 'outline'}
          aria-pressed={tab === id}
          onClick={() => {
            setTab(id)
          }}
        >
          {t(`viewer.keys.tab_${id}`)}
        </Button>
      ))}
    </div>
  )

  if (tab === 'info') {
    return (
      <div className="space-y-3">
        {tabs}
        <RedisInfoPanel databaseName={databaseName} />
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {tabs}
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
        <label className="space-y-1 text-xs text-muted-foreground">
          <span>{t('viewer.keys.typeFilter')}</span>
          <select
            className="block h-9 rounded-md border border-border bg-background px-2 text-sm"
            value={typeFilter}
            onChange={(e) => {
              setTypeFilter(e.target.value)
            }}
          >
            <option value="">{t('viewer.keys.allTypes')}</option>
            {types.map((ty) => (
              <option key={ty} value={ty}>
                {ty}
              </option>
            ))}
          </select>
        </label>
        <Button type="submit" size="sm">
          <SearchGlassIcon className="size-5" aria-hidden="true" />
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
            <p className="rounded-lg border border-border bg-muted/30 px-4 py-4 text-center text-sm text-muted-foreground">
              {t('viewer.keys.empty')}
            </p>
          ) : (
            <ul className="max-h-[28rem] divide-y divide-border/60 overflow-auto rounded-lg border border-border">
              {visible.map((k) => (
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
              <RedisValuePanel value={value.data} />
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}
