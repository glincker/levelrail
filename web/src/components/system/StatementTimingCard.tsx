import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError, readErrorMessage } from '@/lib/apiError'
import { pollMs } from '@/lib/pollTier'

interface SlowStatement {
  sql: string
  max_ms: number
  last_ms: number
  count: number
  last_seen_at: string
}

interface SlowStatements {
  enabled: boolean
  slow_threshold_ms: number
  ring_size: number
  statements: SlowStatement[]
}

const URL = '/api/v1/system/slow-statements'

function useSlowStatements() {
  return useQuery({
    queryKey: ['system-slow-statements'],
    queryFn: async ({ signal }): Promise<SlowStatements> => {
      const res = await fetch(URL, { signal, cache: 'no-store' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch slow statements failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as SlowStatements
    },
    refetchInterval: pollMs('steady'),
    retry: false,
  })
}

// Root only on the server: a 403 hides the card instead of showing an error.
export function StatementTimingCard() {
  const { t } = useTranslation('settings', {
    keyPrefix: 'controlPlaneHealth.statementTiming',
  })
  const { data, isPending, error } = useSlowStatements()
  if (error instanceof ApiError && error.status === 403) return null
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <DatabaseGlassIcon size="1.4em" aria-hidden="true" />
            {t('title')}
          </CardTitle>
          {data ? (
            <Badge variant={data.enabled ? 'success' : 'muted'}>
              {data.enabled ? t('on') : t('off')}
            </Badge>
          ) : null}
        </div>
        {data ? (
          <p className="text-sm text-muted-foreground">
            {t('basis', {
              threshold: data.slow_threshold_ms,
              size: data.ring_size,
            })}
          </p>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        {isPending ? <Skeleton className="h-16 w-full" /> : null}
        {error && !(error instanceof ApiError && error.status === 403) ? (
          <p className="text-sm text-destructive">{t('loadError')}</p>
        ) : null}
        {data && !data.enabled ? (
          <p className="text-sm text-muted-foreground">{t('offHint')}</p>
        ) : null}
        {data && data.enabled && data.statements.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('empty')}</p>
        ) : null}
        {data && data.statements.length > 0 ? (
          <ul className="space-y-3">
            {data.statements.map((s) => (
              <li key={s.sql} className="space-y-1">
                <code className="block break-all font-mono text-xs">
                  {s.sql}
                </code>
                <p className="text-xs tabular-nums text-muted-foreground">
                  {t('row', {
                    max: s.max_ms.toFixed(1),
                    last: s.last_ms.toFixed(1),
                    count: s.count,
                  })}
                </p>
              </li>
            ))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
