import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { domainsQueryOptions } from '../../queries/domains'
import { useDomainPolicies } from '../../queries/domainPolicies'
import { domainScope } from '../../lib/standaloneDomain'
import { ShieldTab } from '../domain/ShieldTab'
import { LoadError } from './LoadError'

/** DomainLimitsCard is the per-domain abuse shield (rate, concurrency, body size, bots, allowlist), shown when the firewall is scoped to one host. */
export function DomainLimitsCard({ domain }: { domain: string }) {
  const { t } = useTranslation('firewall')
  const domains = useQuery(domainsQueryOptions())
  const row = domains.data?.find((d) => d.domain === domain)
  const app = row ? domainScope(row) : ''
  const policies = useDomainPolicies(app, domain)

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('limits.title', { domain })}</CardTitle>
        <CardDescription>{t('limits.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {domains.error ? (
          <LoadError
            compact
            error={domains.error}
            onRetry={() => void domains.refetch()}
          />
        ) : !row && domains.data ? (
          <p className="text-sm text-muted-foreground">
            {t('limits.unknownDomain')}
          </p>
        ) : policies.error ? (
          <LoadError
            compact
            error={policies.error}
            onRetry={() => void policies.refetch()}
          />
        ) : policies.data ? (
          <ShieldTab app={app} domain={domain} policies={policies.data} />
        ) : (
          <div aria-hidden="true" className="space-y-3">
            <Skeleton className="h-8 w-full max-w-xl" />
            <Skeleton className="h-40 w-full" />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
