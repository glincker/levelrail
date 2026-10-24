import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useRuntimeSettings } from '../../queries/runtimeSettings'

type KnobLabel =
  | 'overview.items.revokedTokens'
  | 'overview.items.expiredTokens'
  | 'overview.items.auditLog'
  | 'overview.items.emailLog'
  | 'overview.items.eventWebhooks'
  | 'overview.items.webhookDeliveries'
  | 'overview.items.alertHistory'
  | 'overview.items.inboxArchive'
  | 'overview.items.deployAttempts'
  | 'overview.items.metricsAndLogs'
  | 'overview.items.pipelineArtifacts'

interface RetentionKnob {
  label: KnobLabel
  /** Runtime setting key when the value is editable in this dashboard. */
  setting?: string
  /** Environment variable that sets it when there is no setting. */
  env?: string
  to?:
    | '/settings/tokens'
    | '/settings/audit-log'
    | '/settings/email-log'
    | '/settings/event-webhooks'
    | '/settings/janitor'
    | '/settings/observability'
}

// Every retention or cleanup knob that exists today, so none hides in an env file.
const KNOBS: RetentionKnob[] = [
  {
    label: 'overview.items.revokedTokens',
    setting: 'retention.revoked_token_days',
    to: '/settings/tokens',
  },
  {
    label: 'overview.items.expiredTokens',
    setting: 'retention.expired_token_days',
    to: '/settings/tokens',
  },
  {
    label: 'overview.items.auditLog',
    env: 'APP_AUDIT_LOG_RETENTION_DAYS',
    to: '/settings/audit-log',
  },
  {
    label: 'overview.items.emailLog',
    env: 'APP_EMAIL_LOG_RETENTION_DAYS',
    to: '/settings/email-log',
  },
  {
    label: 'overview.items.eventWebhooks',
    env: 'APP_EVENT_WEBHOOK_RETENTION',
    to: '/settings/event-webhooks',
  },
  {
    label: 'overview.items.webhookDeliveries',
    env: 'APP_WEBHOOK_DELIVERY_RETENTION_DAYS',
  },
  { label: 'overview.items.alertHistory', env: 'APP_ALERT_HISTORY_RETENTION' },
  { label: 'overview.items.inboxArchive', env: 'APP_INBOX_ARCHIVE_RETENTION' },
  {
    label: 'overview.items.deployAttempts',
    to: '/settings/janitor',
  },
  {
    label: 'overview.items.metricsAndLogs',
    env: 'APP_METRICS_RETENTION',
    to: '/settings/observability',
  },
  {
    label: 'overview.items.pipelineArtifacts',
    env: 'APP_PIPELINE_ARTIFACT_RETENTION',
  },
]

export function RetentionOverviewCard() {
  const { t } = useTranslation('runtimeSettings')
  const { data } = useRuntimeSettings()
  const valueOf = (key: string): string | null => {
    const s = data?.settings.find((x) => x.key === key)
    if (!s) return null
    const days = Number(s.value)
    return days === 0
      ? t('retention.never')
      : t('retention.days', { count: days })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('overview.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('overview.intro')}</p>
      </CardHeader>
      <CardContent>
        <ul className="divide-y divide-border text-sm">
          {KNOBS.map((k) => {
            const current = k.setting ? valueOf(k.setting) : null
            return (
              <li
                key={k.label}
                className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-2"
              >
                <span>{t(k.label)}</span>
                <span className="flex items-center gap-3 text-xs text-muted-foreground">
                  {current ? (
                    <span className="text-foreground">{current}</span>
                  ) : k.env ? (
                    <span>{t('row.envHint', { env: k.env })}</span>
                  ) : (
                    <span>{t('overview.policy')}</span>
                  )}
                  {k.to ? (
                    <Link
                      to={k.to}
                      className="text-primary underline underline-offset-4 hover:no-underline"
                    >
                      {t('overview.open')}
                    </Link>
                  ) : null}
                </span>
              </li>
            )
          })}
        </ul>
      </CardContent>
    </Card>
  )
}
