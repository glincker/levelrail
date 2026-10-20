import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  useFixSnapshot,
  useResilience,
  useSimulateOutage,
  type ResilienceCheck,
  type ResilienceStatus,
} from '../../queries/outageResilience'

const BADGE: Record<
  ResilienceStatus,
  'success' | 'destructive' | 'warning' | 'muted'
> = {
  pass: 'success',
  fail: 'destructive',
  warn: 'warning',
  unknown: 'muted',
}

function CheckRow({
  check,
  onFix,
  fixing,
}: Readonly<{ check: ResilienceCheck; onFix: () => void; fixing: boolean }>) {
  const { t } = useTranslation('nodes')
  return (
    <li className="flex flex-wrap items-start justify-between gap-2 py-2">
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex items-center gap-2 text-sm font-medium">
          {t(`resilience.checks.${check.id}`, { defaultValue: check.id })}
          <Badge variant={BADGE[check.status]}>
            {t(`resilience.status.${check.status}`)}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground">{check.detail}</p>
      </div>
      {check.fix ? (
        <Button size="sm" onClick={onFix} disabled={fixing}>
          {t('resilience.fix')}
        </Button>
      ) : null}
    </li>
  )
}

export function ResilienceCard() {
  const { t } = useTranslation('nodes')
  const query = useResilience()
  const simulate = useSimulateOutage()
  const fix = useFixSnapshot()
  const report = query.data
  const error = query.error ?? simulate.error ?? fix.error

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{t('resilience.title')}</CardTitle>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            {t('resilience.recheck')}
          </Button>
          <Button
            size="sm"
            onClick={() => simulate.mutate()}
            disabled={simulate.isPending}
          >
            {t('resilience.simulate')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('resilience.description')}
        </p>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        ) : null}
        {report?.simulated ? (
          <p className="text-sm text-muted-foreground">
            {t('resilience.simulatedNote')}
          </p>
        ) : null}
        {report?.scopes.map((scope) => (
          <section key={scope.id} aria-label={scope.name}>
            <h3 className="text-sm font-semibold">{scope.name}</h3>
            <ul className="divide-y divide-border">
              {scope.checks.map((c) => (
                <CheckRow
                  key={c.id}
                  check={c}
                  onFix={() => fix.mutate()}
                  fixing={fix.isPending}
                />
              ))}
            </ul>
          </section>
        ))}
      </CardContent>
    </Card>
  )
}
