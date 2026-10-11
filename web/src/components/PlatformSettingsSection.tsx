import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CaretRightIcon, GearSixIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { CertificateStatus } from '../queries/certificates'
import type { CloudflareDnsSettings } from '../queries/cloudflareDns'
import type { IngressSettings } from '../queries/domains'
import type { Route53DnsSettings } from '../queries/route53Dns'
import { AppsBaseDomainCard } from './AppsBaseDomainCard'
import { CloudflareDnsCard } from './CloudflareDnsCard'
import { DomainAutomationCard } from './DomainAutomationCard'
import { DashboardUrlCard } from './DashboardUrlCard'
import { EnableHttpsCard } from './EnableHttpsCard'
import { FallbackDomainsCard } from './FallbackDomainsCard'
import { IngressConnectivityCard } from './IngressConnectivityCard'
import { IngressSettingsCard } from './IngressSettingsCard'
import { ReverseProxyGuideCard } from './ReverseProxyGuideCard'
import { Route53DnsCard } from './Route53DnsCard'

// Closed by default: these are set once per platform, not per domain.
// Panel children only mount when open, so closed costs no requests.
export function PlatformSettingsSection({
  settings,
  primaryCert,
  cloudflareDns,
  route53Dns,
}: {
  settings: IngressSettings
  primaryCert?: CertificateStatus
  cloudflareDns: CloudflareDnsSettings
  route53Dns: Route53DnsSettings
}) {
  const { t } = useTranslation('domains')
  const [open, setOpen] = useState(false)
  const httpsOff =
    !settings.primary_domain || settings.primary_domain.endsWith('.sslip.io')

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg border border-border bg-card"
    >
      <CollapsibleTrigger className="flex items-center gap-3 px-4 py-3">
        <CaretRightIcon
          className={
            open
              ? 'size-4 shrink-0 rotate-90 transition-transform'
              : 'size-4 shrink-0 transition-transform'
          }
          aria-hidden="true"
        />
        <GearSixIcon
          className="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-semibold text-foreground">
            {t('page.platform.title')}
          </span>
          <span className="block truncate text-xs text-muted-foreground">
            {t('page.platform.summary')}
          </span>
        </span>
        {httpsOff ? (
          <Badge variant="warning">{t('page.platform.httpsOff')}</Badge>
        ) : null}
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="space-y-4 border-t border-border p-4">
          {httpsOff ? <EnableHttpsCard /> : null}
          <IngressSettingsCard settings={settings} primaryCert={primaryCert} />
          <DashboardUrlCard />
          <DomainAutomationCard />
          <AppsBaseDomainCard />
          <FallbackDomainsCard />
          <IngressConnectivityCard />
          <ReverseProxyGuideCard />
          <CloudflareDnsCard settings={cloudflareDns} />
          <Route53DnsCard settings={route53Dns} />
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
