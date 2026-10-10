import { useTranslation } from 'react-i18next'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type { DomainCheckStatus } from '../queries/domainCheck'

const VARIANT: Record<
  DomainCheckStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  connected: 'success',
  not_resolving: 'warning',
  resolves_elsewhere: 'destructive',
  unconfigured: 'muted',
}

export function DomainDnsStatusBadge({
  status,
}: {
  status?: DomainCheckStatus
}) {
  const { t } = useTranslation('domains')
  if (!status) {
    return <Badge variant="muted">{t('page.dns.checking')}</Badge>
  }
  return <Badge variant={VARIANT[status]}>{t(`page.dns.${status}`)}</Badge>
}
