import { useTranslation } from 'react-i18next'
import { Sparkline } from '@/components/kit'
import type { LiveUpstream } from '../../queries/loadBalancerLive'

function fmt(n: number): string {
  if (n >= 100) return String(Math.round(n))
  return n >= 10 ? n.toFixed(1) : n.toFixed(2).replace(/\.?0+$/, '') || '0'
}

function formatErrorRate(rate: number): string {
  const pct = rate * 100
  return `${pct >= 10 ? Math.round(pct) : Number(pct.toFixed(1))}%`
}

export function LbTrafficCell({ u }: { u: LiveUpstream }) {
  const { t } = useTranslation('common')
  const traffic = u.traffic
  if (!traffic) {
    return (
      <span
        className="text-xs text-muted-foreground"
        title={t('lbTraffic.emptyHint')}
      >
        {t('lbTraffic.empty')}
      </span>
    )
  }
  const failing = traffic.error_rate > 0
  return (
    <div className="min-w-0 space-y-0.5" data-testid="lb-traffic">
      <span className="flex items-center gap-2">
        {traffic.rps_series.length >= 2 ? (
          <Sparkline
            values={traffic.rps_series}
            tone="info"
            width={48}
            height={18}
            ariaLabel={t('lbTraffic.sparkline', {
              minutes: Math.max(1, Math.round(traffic.window_seconds / 60)),
              dial: u.dial,
            })}
          />
        ) : null}
        <span className="text-xs font-medium tabular-nums">
          {t('lbTraffic.rps', { value: fmt(traffic.requests_per_sec) })}
        </span>
      </span>
      <p className="truncate text-xs text-muted-foreground tabular-nums">
        <span className={failing ? 'text-destructive' : undefined}>
          {t('lbTraffic.errors', {
            value: formatErrorRate(traffic.error_rate),
          })}
        </span>
        {' · '}
        {t('lbTraffic.p50', { value: Math.round(traffic.p50_ms) })}
        {' · '}
        {t('lbTraffic.p95', { value: Math.round(traffic.p95_ms) })}
      </p>
    </div>
  )
}
