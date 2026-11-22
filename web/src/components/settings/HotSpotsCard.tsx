import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type {
  CPHealthHotRoute,
  CPHealthHotStatement,
} from '../../queries/controlPlaneHealth'

export function HotSpotsCard({
  routes,
  statements,
}: {
  routes: CPHealthHotRoute[]
  statements: CPHealthHotStatement[]
}) {
  const { t } = useTranslation('hotSpots')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('routesTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {routes.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('noRoutes')}</p>
        ) : (
          routes.map((r) => (
            <div key={`${r.method} ${r.route}`} className="space-y-0.5 text-xs">
              <code className="break-all">
                {r.method} {r.route}
              </code>
              <p className="text-muted-foreground tabular-nums">
                {t('routeRow', {
                  count: r.count,
                  errors: r.errors,
                  avg: r.avg_ms.toFixed(1),
                  max: r.max_ms.toFixed(0),
                  total: r.total_ms.toFixed(0),
                })}
              </p>
            </div>
          ))
        )}
        <h3 className="pt-2 text-sm font-medium">{t('statementsTitle')}</h3>
        {statements.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('noStatements')}</p>
        ) : (
          statements.map((q) => (
            <div key={q.sql} className="space-y-0.5 text-xs">
              <code className="break-all">{q.sql}</code>
              <p className="text-muted-foreground tabular-nums">
                {t('statementRow', {
                  count: q.count,
                  avg: q.avg_ms.toFixed(2),
                  max: q.max_ms.toFixed(0),
                  total: q.total_ms.toFixed(0),
                })}
              </p>
            </div>
          ))
        )}
        <p className="text-xs text-muted-foreground">{t('hint')}</p>
      </CardContent>
    </Card>
  )
}
