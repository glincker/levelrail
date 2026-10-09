import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { DnsProvider } from '../queries/domainCheck'

// The DNS-01 provider the ingress would use for wildcard certificates.
export function DnsProviderBadge({ provider }: { provider: DnsProvider }) {
  const { t } = useTranslation('domains')
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
      {t('provider.label')}:
      <Badge variant={provider === 'none' ? 'muted' : 'success'}>
        {t(`provider.${provider}`)}
      </Badge>
    </span>
  )
}
