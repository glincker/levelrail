import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useIngressConnectivity } from '../queries/domainWizard'
import { DnsProviderBadge } from './DnsProviderBadge'
import { ProxyManagedCertNote } from './ProxyManagedCertNote'

// Whether ports 80 and 443 answer on this server's address, and which
// DNS-01 provider is connected: the two facts that decide HTTP-01 vs DNS-01.
export function IngressConnectivityCard() {
  const { t } = useTranslation('domains')
  const { data, isFetching, refetch } = useIngressConnectivity()
  const guidanceKey = {
    ok: 'connectivityCard.ok',
    no_host: 'connectivityCard.noHost',
    private_address: 'connectivityCard.private',
    ports_unreachable: 'connectivityCard.closed',
  } as const

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugGlassIcon className="size-4.5" aria-hidden="true" />
          {t('connectivityCard.title')}
        </CardTitle>
        <CardDescription>{t('connectivityCard.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {data ? (
          <>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              {data.host ? (
                <span className="font-mono">{data.host}</span>
              ) : null}
              {data.ports.map((p) => (
                <Badge
                  key={p.port}
                  variant={p.reachable ? 'success' : 'warning'}
                >
                  {p.reachable
                    ? t('certificate.portOpen', { port: p.port })
                    : t('certificate.portClosed', { port: p.port })}
                </Badge>
              ))}
            </div>
            <p className="text-sm text-foreground">
              {t(guidanceKey[data.guidance])}
            </p>
            {data.tls_upstream ? (
              <ProxyManagedCertNote />
            ) : (
              <DnsProviderBadge provider={data.dns_provider} />
            )}
            {data.dns_provider === 'none' && !data.tls_upstream ? (
              <p className="text-xs text-muted-foreground">
                {t('provider.noneHint')}
              </p>
            ) : null}
          </>
        ) : (
          <Skeleton className="h-10 w-full" />
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={isFetching}
          onClick={() => {
            void refetch()
          }}
        >
          <RefreshCwGlassIcon
            className={isFetching ? 'size-4 animate-spin' : 'size-4'}
            aria-hidden="true"
          />
          {t('connectivityCard.check')}
        </Button>
      </CardContent>
    </Card>
  )
}
