import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  ListMagnifyingGlassIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import { useEmailStatus } from '../../queries/emailSettings'

function Stat({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0 space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm font-medium break-words">{children}</div>
    </div>
  )
}

/** EmailStatusHeader says whether sending works: configured, last success, last failure. */
export function EmailStatusHeader() {
  const { t } = useTranslation('settings')
  const { data: status } = useEmailStatus()
  if (!status) return null

  const never = t('emailDelivery.status.never')
  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          {status.configured ? (
            <Badge variant="success">
              <CheckCircleIcon className="size-3" aria-hidden="true" />
              {t('emailDelivery.status.configured')}
            </Badge>
          ) : (
            <Badge variant="muted">{t('emailPage.status.none')}</Badge>
          )}
          {status.provider ? (
            <span className="text-sm text-muted-foreground">
              {status.provider === 'env'
                ? t('emailDelivery.status.fromEnv')
                : t(
                    `emailDelivery.providers.${status.provider}` as 'emailDelivery.providers.smtp',
                  )}
            </span>
          ) : null}
          <Link
            to="/settings/email-log"
            className={buttonVariants({
              variant: 'outline',
              size: 'sm',
              className: 'ml-auto',
            })}
          >
            <ListMagnifyingGlassIcon className="size-3.5" aria-hidden="true" />
            {t('emailDelivery.status.openLog')}
          </Link>
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Stat label={t('emailDelivery.status.lastSuccess')}>
            {status.last_success_at ? (
              <span title={formatAbsoluteTime(status.last_success_at)}>
                {formatRelativeTime(status.last_success_at)}
              </span>
            ) : (
              never
            )}
          </Stat>
          <Stat label={t('emailDelivery.status.lastFailure')}>
            {status.last_failure_at ? (
              <span
                className="inline-flex items-center gap-1 text-tone-danger"
                title={formatAbsoluteTime(status.last_failure_at)}
              >
                <WarningCircleIcon className="size-3.5" aria-hidden="true" />
                {formatRelativeTime(status.last_failure_at)}
                {status.last_error_kind
                  ? `: ${t(`emailDelivery.failure.${status.last_error_kind as 'other'}.title`)}`
                  : ''}
              </span>
            ) : (
              never
            )}
          </Stat>
          <Stat label={t('emailDelivery.status.sent24h')}>
            {status.sent_24h}
          </Stat>
          <Stat label={t('emailDelivery.status.failed24h')}>
            {status.failed_24h}
          </Stat>
        </div>
      </CardContent>
    </Card>
  )
}
