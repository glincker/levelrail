import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { TrafficErrorSpike } from '../../types/trafficDetail'
import { formatPercent } from './format'

/** ErrorSpikesCard lists 5xx spikes in the window and the deploy that preceded each. */
export function ErrorSpikesCard({
  spikes,
}: Readonly<{ spikes: TrafficErrorSpike[] }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('spikes.title')}</CardTitle>
        <p className="text-xs text-muted-foreground">{t('spikes.hint')}</p>
      </CardHeader>
      <CardContent>
        {spikes.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('spikes.none')}</p>
        ) : (
          <ul className="space-y-3">
            {spikes.map((s) => (
              <li
                key={s.start}
                className="flex flex-wrap items-start gap-x-3 gap-y-1 text-sm"
              >
                <TriangleAlertGlassIcon
                  className="mt-0.5 size-4.5 text-destructive"
                  aria-hidden
                />
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="font-medium">
                    {t('spikes.peak', {
                      rate: formatPercent(s.peak_error_rate_5xx),
                    })}
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      {new Date(s.start).toLocaleString()}
                    </span>
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('spikes.detail', {
                      errors: Math.round(s.errors_5xx),
                      requests: Math.round(s.requests),
                      baseline: formatPercent(s.baseline_error_rate_5xx),
                    })}
                  </p>
                </div>
                {s.deploy_id ? (
                  <Badge variant="outline" className="max-w-full truncate">
                    {t('spikes.afterDeploy', {
                      image: s.deploy_image ?? s.deploy_id,
                      time: s.deploy_at
                        ? new Date(s.deploy_at).toLocaleString()
                        : '',
                    })}
                  </Badge>
                ) : (
                  <Badge variant="muted">{t('spikes.noDeploy')}</Badge>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
