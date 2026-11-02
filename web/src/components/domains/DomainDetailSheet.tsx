import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { DomainAuthGateControl } from '../DomainAuthGateControl'
import { DomainBasicAuthControl } from '../DomainBasicAuthControl'
import { DomainDnsCheck } from '../DomainDnsCheck'
import { DomainDnsRecordsControl } from '../DomainDnsRecordsControl'
import { DomainErrorPagesControl } from '../DomainErrorPagesControl'
import { DomainMaintenanceControl } from '../DomainMaintenanceControl'
import { DomainRedirectControl } from '../DomainRedirectControl'
import { DomainSearchVisibilityControl } from '../DomainSearchVisibilityControl'
import { DomainTLSCertControl } from '../DomainTLSCertControl'
import { DomainWafControl } from '../DomainWafControl'
import { ProxyDomainStatus } from '../ProxyDomainStatus'

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {title}
      </h3>
      <div className="space-y-2">{children}</div>
    </section>
  )
}

// Per-domain settings grouped by what they do, all in one place so the
// list page itself stays one row per domain.
export function DomainDetailSheet({
  appName,
  domain,
  open,
  onOpenChange,
}: {
  appName: string
  domain: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full space-y-5 overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle className="font-mono">{domain}</SheetTitle>
          <SheetDescription>
            {t('detail.description', { app: appName })}{' '}
            <Link
              to="/domains/$domain"
              params={{ domain }}
              search={{ app: appName }}
              className="underline"
            >
              {t('detail.allSettings')}
            </Link>
          </SheetDescription>
        </SheetHeader>
        <div className="space-y-5 px-4 pb-6">
          <Group title={t('detail.routing')}>
            <DomainDnsCheck appName={appName} domain={domain} />
            <ProxyDomainStatus appName={appName} domain={domain} />
            <DomainDnsRecordsControl appName={appName} domain={domain} />
            <DomainRedirectControl appName={appName} domain={domain} />
          </Group>
          <Group title={t('detail.security')}>
            <DomainBasicAuthControl appName={appName} domain={domain} />
            <DomainAuthGateControl appName={appName} domain={domain} />
            <DomainWafControl appName={appName} domain={domain} />
          </Group>
          <Group title={t('detail.pages')}>
            <DomainMaintenanceControl appName={appName} domain={domain} />
            <DomainErrorPagesControl appName={appName} domain={domain} />
          </Group>
          <Group title={t('detail.seo')}>
            <DomainSearchVisibilityControl appName={appName} domain={domain} />
          </Group>
          <Group title={t('detail.tls')}>
            <DomainTLSCertControl appName={appName} domain={domain} />
          </Group>
        </div>
      </SheetContent>
    </Sheet>
  )
}
