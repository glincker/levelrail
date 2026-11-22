import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '../../lib/format'
import { formatMoney } from '../../lib/usageFormat'
import { useAdvisorFindings } from '../../queries/resourceAdvisor'
import { idleAppFindings } from '../../lib/idleApps'

const SAVING_CURRENCY = 'USD'

export function CostIdleApps() {
  const { t } = useTranslation('costIdle')
  const query = useAdvisorFindings()
  if (query.isError) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {t('error', { message: query.error.message })}
      </p>
    )
  }
  const idle = idleAppFindings(query.data?.findings ?? [])
  if (idle.length === 0) return null
  const total = idle.reduce((sum, f) => sum + (f.monthly_saving_usd ?? 0), 0)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
        {total > 0 ? (
          <p className="text-sm font-medium tabular-nums">
            {t('total', { amount: formatMoney(total, SAVING_CURRENCY) })}
          </p>
        ) : null}
      </CardHeader>
      <CardContent>
        <ul className="divide-y">
          {idle.map((f) => (
            <li
              key={f.id}
              className="flex flex-wrap items-center justify-between gap-2 py-2"
            >
              <Link
                to="/apps/$name"
                params={{ name: f.resource.name }}
                className="font-medium hover:underline"
              >
                {f.resource.name}
              </Link>
              <span className="flex flex-wrap items-center gap-x-4 text-sm text-muted-foreground tabular-nums">
                {f.impact.saving_memory_bytes ? (
                  <span>
                    {t('memory', {
                      memory: formatBytes(f.impact.saving_memory_bytes),
                    })}
                  </span>
                ) : null}
                {f.monthly_saving_usd ? (
                  <span className="text-foreground">
                    {t('saving', {
                      amount: formatMoney(
                        f.monthly_saving_usd,
                        SAVING_CURRENCY,
                      ),
                    })}
                  </span>
                ) : null}
              </span>
            </li>
          ))}
        </ul>
        <Button
          className="mt-3"
          size="sm"
          variant="ghost"
          render={<Link to="/resource-advisor" />}
        >
          {t('review')}
        </Button>
      </CardContent>
    </Card>
  )
}
