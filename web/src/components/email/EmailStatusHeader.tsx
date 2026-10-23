import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowsLeftRightIcon,
  ListMagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { formatAbsoluteTime, formatRelativeTime } from '@/lib/relativeTime'
import { providerByRole, useEmailProviders } from '../../queries/emailProviders'
import { useEmailStatus } from '../../queries/emailSettings'
import { ProviderStatusChip } from './EmailProvidersCard'
import { CARD_FLOAT, PRESS } from './emailStyles'
import { isRunningOnBackup } from './providerHealth'

function Stat({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="text-sm font-medium break-words">{children}</div>
    </div>
  )
}

/** EmailStatusHeader is one compact bar: primary and backup state, running-on-backup, last send and last error. */
export function EmailStatusHeader() {
  const { t } = useTranslation('settings')
  const { data: providers } = useEmailProviders()
  const { data: status } = useEmailStatus()
  const primary = providerByRole(providers, 'primary')
  const backup = providerByRole(providers, 'backup')
  const onBackup = isRunningOnBackup(primary, backup)
  const never = t('emailDelivery.status.never')
  const lastOk = [primary?.last_ok_at, backup?.last_ok_at]
    .filter((v): v is string => Boolean(v))
    .sort()
    .at(-1)
  const errorKind = primary?.last_error_kind || backup?.last_error_kind

  return (
    <Card size="sm" className={CARD_FLOAT}>
      <CardContent className="flex flex-wrap items-center gap-x-6 gap-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <Stat label={t('emailProviders.roles.primary')}>
            {primary ? (
              <ProviderStatusChip provider={primary} />
            ) : (
              <Badge variant="muted">{t('emailProviders.health.none')}</Badge>
            )}
          </Stat>
          <Stat label={t('emailProviders.roles.backup')}>
            {backup ? (
              <ProviderStatusChip provider={backup} />
            ) : (
              <Badge variant="muted">{t('emailProviders.health.none')}</Badge>
            )}
          </Stat>
          {onBackup ? (
            <Badge variant="warning" role="status">
              <ArrowsLeftRightIcon className="size-3" aria-hidden="true" />
              {t('emailProviders.runningOnBackup')}
            </Badge>
          ) : null}
        </div>
        <Stat label={t('emailDelivery.status.lastSuccess')}>
          {lastOk ? (
            <span title={formatAbsoluteTime(lastOk)}>
              {formatRelativeTime(lastOk)}
            </span>
          ) : (
            never
          )}
        </Stat>
        <Stat label={t('emailProviders.status.lastError')}>
          {errorKind ? (
            <span className="text-tone-danger">
              {t(`emailDelivery.failure.${errorKind as 'other'}.title`)}
            </span>
          ) : (
            t('emailProviders.status.noError')
          )}
        </Stat>
        {status ? (
          <>
            <Stat label={t('emailDelivery.status.sent24h')}>
              {status.sent_24h}
            </Stat>
            <Stat label={t('emailDelivery.status.failed24h')}>
              {status.failed_24h}
            </Stat>
          </>
        ) : null}
        <Link
          to="/settings/email-log"
          className={buttonVariants({
            variant: 'outline',
            size: 'sm',
            className: `ml-auto ${PRESS}`,
          })}
        >
          <ListMagnifyingGlassIcon className="size-3.5" aria-hidden="true" />
          {t('emailDelivery.status.openLog')}
        </Link>
      </CardContent>
    </Card>
  )
}
