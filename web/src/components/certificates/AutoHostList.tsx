import { useState } from 'react'
import { useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { appListQueryOptions } from '../../queries/apps'
import {
  autoHostQueryOptions,
  certBudgetKeys,
  useSetAutoHost,
  type AutoHost,
} from '../../queries/certBudget'

// UI sizing, not a policy threshold: when to show search, and rows per page.
const SEARCH_AFTER = 10
const PAGE_SIZE = 50

export function AutoHostList() {
  const { t } = useTranslation('certBudget')
  const queryClient = useQueryClient()
  const apps = useQuery(appListQueryOptions())
  const setAutoHost = useSetAutoHost()
  const [search, setSearch] = useState('')
  const [limit, setLimit] = useState(PAGE_SIZE)

  const all = apps.data ?? []
  const needle = search.trim().toLowerCase()
  const filtered = needle
    ? all.filter((a) => a.name.toLowerCase().includes(needle))
    : all
  const shown = filtered.slice(0, limit)

  const states = useQueries({
    queries: shown.map((a) => autoHostQueryOptions(a.name)),
  })

  const toggle = (app: string, enabled: boolean) => {
    const key = certBudgetKeys.autoHost(app)
    const previous = queryClient.getQueryData<AutoHost>(key)
    queryClient.setQueryData<AutoHost>(key, { enabled })
    setAutoHost.mutate(
      { app, enabled },
      {
        onError: () => {
          queryClient.setQueryData(key, previous)
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-3">
        <CardTitle>{t('autoHost.title')}</CardTitle>
        {all.length > SEARCH_AFTER ? (
          <Input
            type="search"
            className="w-56"
            value={search}
            aria-label={t('autoHost.search')}
            placeholder={t('autoHost.search')}
            onChange={(e) => {
              setSearch(e.target.value)
              setLimit(PAGE_SIZE)
            }}
          />
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">{t('autoHost.help')}</p>
        {apps.isError ? (
          <div className="flex flex-wrap items-center gap-3" role="alert">
            <span className="text-sm text-tone-danger">
              {t('autoHost.loadError')}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void apps.refetch()}
            >
              {t('autoHost.retry')}
            </Button>
          </div>
        ) : apps.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            …
          </p>
        ) : all.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('autoHost.empty')}</p>
        ) : filtered.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('autoHost.noMatch')}
          </p>
        ) : (
          <>
            <ul className="divide-y divide-border">
              {shown.map((a, i) => {
                const s = states[i]?.data
                return (
                  <li
                    key={a.name}
                    className="flex items-center justify-between gap-3 py-2"
                  >
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium">
                        {a.name}
                      </div>
                      <div className="truncate text-xs text-muted-foreground">
                        {s?.enabled
                          ? (s.hostname ?? t('autoHost.on'))
                          : s
                            ? t('autoHost.off')
                            : ''}
                      </div>
                    </div>
                    <Switch
                      size="sm"
                      checked={s?.enabled ?? false}
                      disabled={!s}
                      aria-label={t('autoHost.toggle', { name: a.name })}
                      onCheckedChange={(v) => {
                        toggle(a.name, v)
                      }}
                    />
                  </li>
                )
              })}
            </ul>
            {filtered.length > shown.length ? (
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs text-muted-foreground">
                  {t('autoHost.showing', {
                    shown: shown.length,
                    total: filtered.length,
                  })}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setLimit((n) => n + PAGE_SIZE)
                  }}
                >
                  {t('autoHost.more')}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  )
}
