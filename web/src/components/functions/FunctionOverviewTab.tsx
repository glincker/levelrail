import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useFunctions } from '../../queries/functions'
import { useFunctionSettings } from '../../queries/functionControls'

function Stat({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="space-y-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium">{value}</dd>
    </div>
  )
}

export function FunctionOverviewTab({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('functions')
  const { data } = useFunctions('24h')
  const { data: settings } = useFunctionSettings(name)
  const fn = data?.functions.find((f) => f.name === name)

  if (!fn) {
    return (
      <p className="text-sm text-muted-foreground">{t('detail.notFound')}</p>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t('detail.overview.title')}
          <Badge variant={fn.sleeping ? 'muted' : 'default'}>
            {fn.sleeping ? t('list.asleep') : t('list.awake')}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-2 gap-4 md:grid-cols-4">
          <Stat label={t('list.calls')} value={String(fn.invocations)} />
          <Stat label={t('list.p95')} value={`${Math.round(fn.p95_ms)} ms`} />
          <Stat
            label={t('list.errors')}
            value={`${(fn.error_rate_5xx * 100).toFixed(1)}%`}
          />
          <Stat
            label={t('list.idle')}
            value={t('list.idleValue', { minutes: fn.idle_minutes })}
          />
          <Stat label={t('detail.settings.port')} value={String(fn.port)} />
          <Stat
            label={t('detail.settings.timeout')}
            value={t('detail.seconds', {
              count: settings?.effective_timeout_seconds ?? 0,
            })}
          />
          <Stat
            label={t('detail.settings.concurrency')}
            value={String(settings?.effective_concurrency ?? 0)}
          />
          <Stat label={t('detail.overview.image')} value={fn.image} />
        </dl>
        <div className="flex flex-wrap gap-4 text-sm">
          {fn.url ? (
            <a
              href={fn.url}
              target="_blank"
              rel="noreferrer"
              className="text-primary hover:underline"
            >
              {fn.url}
            </a>
          ) : null}
          <Link
            to="/apps/$name"
            params={{ name }}
            className="text-primary hover:underline"
          >
            {t('detail.overview.openApp')}
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}
