import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, CardContent } from '@/components/ui/card'
import type { TrafficShield } from '../../types/trafficDetail'

/** ShieldBlockedCard shows requests the ingress abuse shield turned away, by reason. */
export function ShieldBlockedCard({
  app,
  shield,
}: Readonly<{ app: string; shield: TrafficShield }>) {
  const { t } = useTranslation('trafficDetail')
  return (
    <Card>
      <CardContent className="space-y-2 py-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-sm font-medium text-foreground">
            {t('shield.title')}
          </p>
          <Link
            to="/apps/$name/domains"
            params={{ name: app }}
            className="text-sm underline underline-offset-4"
          >
            {t('shield.action')}
          </Link>
        </div>
        {shield.blocked > 0 ? (
          <>
            <p className="text-2xl font-semibold tabular-nums">
              {t('shield.blocked', { count: shield.blocked })}
            </p>
            <ul className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
              {shield.by_reason
                .filter((r) => r.count > 0)
                .map((r) => (
                  <li key={r.reason}>
                    {t(`shield.reasons.${r.reason}`)}:{' '}
                    <span className="tabular-nums">{r.count}</span>
                  </li>
                ))}
            </ul>
          </>
        ) : (
          <p className="text-xs text-muted-foreground">{t('shield.none')}</p>
        )}
      </CardContent>
    </Card>
  )
}
