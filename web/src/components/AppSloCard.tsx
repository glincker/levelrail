import { useTranslation } from 'react-i18next'
import { TargetGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DEFAULT_SLO_TARGET,
  useSloPreview,
  type SloPreview,
} from '../queries/sloPreview'
import type { AlertRule, SloConfig } from '../types/alerts'

const DEFAULT_SLO: SloConfig = {
  objective: 'availability',
  target: DEFAULT_SLO_TARGET,
}

function budgetTone(remaining: number): string {
  if (remaining <= 0.1) {
    return '[&_[data-slot=progress-indicator]]:bg-destructive'
  }
  if (remaining <= 0.4) {
    return '[&_[data-slot=progress-indicator]]:bg-tone-warning-solid'
  }
  return ''
}

function windowLabel(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400}d`
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  return `${Math.round(seconds / 60)}m`
}

function SloStatusBadge({ preview }: { preview: SloPreview }) {
  const { t } = useTranslation('observability')
  if (!preview.has_traffic) {
    return <Badge variant="muted">{t('slo.noTraffic')}</Badge>
  }
  if (preview.page) {
    return <Badge variant="destructive">{t('slo.paging')}</Badge>
  }
  if (preview.firing) {
    return <Badge variant="warning">{t('slo.burning')}</Badge>
  }
  return <Badge variant="success">{t('slo.healthy')}</Badge>
}

function SloRuleRow({ appName, rule }: { appName: string; rule: AlertRule }) {
  const { t } = useTranslation('observability')
  const slo = rule.slo ?? DEFAULT_SLO
  const preview = useSloPreview(appName, slo)

  const objective =
    slo.objective === 'latency'
      ? t('slo.latencyObjective', {
          target: slo.target,
          ms: slo.latency_ms ?? 0,
        })
      : t('slo.availabilityObjective', { target: slo.target })

  return (
    <li className="rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {rule.name}
          </p>
          <p className="text-xs text-muted-foreground">{objective}</p>
        </div>
        {preview.data ? <SloStatusBadge preview={preview.data} /> : null}
      </div>
      {preview.isPending ? (
        <Skeleton className="mt-3 h-4 w-full" />
      ) : preview.isError ? (
        <p className="mt-3 text-xs text-destructive">{preview.error.message}</p>
      ) : preview.data ? (
        <SloRuleDetail preview={preview.data} />
      ) : null}
    </li>
  )
}

function SloRuleDetail({ preview }: { preview: SloPreview }) {
  const { t } = useTranslation('observability')
  if (!preview.has_traffic) {
    return (
      <p className="mt-3 text-xs text-muted-foreground">
        {t('slo.noTrafficHint')}
      </p>
    )
  }
  const remaining = Math.max(0, Math.min(1, preview.budget_remaining))
  return (
    <div className="mt-3 space-y-3">
      <div>
        <div className="mb-1 flex items-center justify-between text-xs">
          <span className="text-muted-foreground">
            {t('slo.budgetRemaining')}
          </span>
          <span className="tabular-nums text-foreground">
            {(remaining * 100).toFixed(1)}%
          </span>
        </div>
        <Progress
          value={remaining * 100}
          aria-label={t('slo.budgetRemaining')}
          className={budgetTone(remaining)}
        />
      </div>
      <ul className="grid gap-1.5 sm:grid-cols-2">
        {preview.tiers.map((tier) => (
          <li
            key={tier.name}
            className="flex items-center justify-between gap-2 rounded-md bg-muted/50 px-2.5 py-1.5 text-xs"
          >
            <span className="text-muted-foreground">
              {t('slo.tierWindow', {
                name: tier.name,
                long: windowLabel(tier.long_seconds),
                short: windowLabel(tier.short_seconds),
              })}
            </span>
            <span className="flex items-center gap-1.5 tabular-nums">
              {t('slo.burnRate', { rate: tier.effective_burn.toFixed(1) })}
              {tier.firing ? (
                <Badge variant={tier.page ? 'destructive' : 'warning'}>
                  {t('slo.firing')}
                </Badge>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** AppSloCard shows error budget left and burn rate for each of an app's SLO rules. */
export function AppSloCard({
  appName,
  rules,
}: {
  appName: string
  rules: AlertRule[]
}) {
  const { t } = useTranslation('observability')
  const sloRules = rules.filter((r) => r.kind === 'slo_burn')
  if (sloRules.length === 0) return null

  return (
    <section className="mt-3 rounded-lg border border-border p-4">
      <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
        <TargetGlassIcon
          className="size-4.5 text-muted-foreground"
          aria-hidden="true"
        />
        {t('slo.title')}
      </h3>
      <ul className="mt-2 space-y-2">
        {sloRules.map((rule) => (
          <SloRuleRow key={rule.id} appName={appName} rule={rule} />
        ))}
      </ul>
    </section>
  )
}
