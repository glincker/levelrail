import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { CPHealthSlowRequest } from '../../queries/controlPlaneHealth'

export function SlowRequestsCard({
  thresholdMs,
  requests,
}: {
  thresholdMs: number
  requests: CPHealthSlowRequest[]
}) {
  const { t } = useTranslation('slowRequests')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title', { ms: Math.round(thresholdMs) })}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        {requests.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('none')}</p>
        ) : (
          requests.slice(0, 10).map((r) => (
            <div
              key={`${r.at}-${r.route}`}
              className="flex items-start justify-between gap-3 text-xs"
            >
              <code className="min-w-0 break-all">{r.route}</code>
              <span className="shrink-0 tabular-nums">
                {r.status} ({Math.round(r.duration_ms)} ms)
              </span>
            </div>
          ))
        )}
        <p className="text-xs text-muted-foreground">{t('hint')}</p>
      </CardContent>
    </Card>
  )
}
