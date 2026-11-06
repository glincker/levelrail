import { useId, useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  RocketGlassIcon,
  StethoscopeGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useAppNetwork } from '../../queries/appNetwork'
import { useDomainAuthGate } from '../../queries/domainAuthGate'
import { useDomainBasicAuth } from '../../queries/domainBasicAuth'
import { useDomainErrorPages } from '../../queries/domainErrorPages'
import { useDomainMaintenance } from '../../queries/domainMaintenance'
import { useDomainRedirect } from '../../queries/domainRedirect'
import { useDomainSearchVisibility } from '../../queries/domainSearchVisibility'
import { useDomainTLSCert } from '../../queries/domainTlsCert'
import { useDomainWaf } from '../../queries/domainWaf'
import { DomainAuthGateControl } from '../DomainAuthGateControl'
import { DomainBasicAuthControl } from '../DomainBasicAuthControl'
import { DomainDnsRecordsControl } from '../DomainDnsRecordsControl'
import { DomainErrorPagesControl } from '../DomainErrorPagesControl'
import { DomainMaintenanceControl } from '../DomainMaintenanceControl'
import { DomainRedirectControl } from '../DomainRedirectControl'
import { DomainSearchVisibilityControl } from '../DomainSearchVisibilityControl'
import { DomainTLSCertControl } from '../DomainTLSCertControl'
import { DomainWafControl } from '../DomainWafControl'
import { DomainDiagnoseDialog } from './DomainDiagnoseDialog'

type ChipKey =
  | 'basicAuth'
  | 'authGate'
  | 'redirect'
  | 'maintenance'
  | 'waf'
  | 'errorPages'
  | 'hidden'
  | 'ownCert'

// Chips come from the same cached queries the controls use, so the
// collapsed header shows what is ON without a second request.
function useActiveChips(appName: string, domain: string): ChipKey[] {
  const basicAuth = useDomainBasicAuth(appName, domain).data
  const authGate = useDomainAuthGate(appName, domain).data
  const redirect = useDomainRedirect(appName, domain).data
  const maintenance = useDomainMaintenance(appName, domain).data
  const waf = useDomainWaf(appName, domain).data
  const errorPages = useDomainErrorPages(appName, domain).data
  const search = useDomainSearchVisibility(appName, domain).data
  const cert = useDomainTLSCert(appName, domain).data
  const chips: [ChipKey, boolean][] = [
    ['basicAuth', basicAuth?.enabled === true],
    ['authGate', authGate?.enabled === true],
    ['redirect', redirect?.enabled === true],
    ['maintenance', maintenance?.enabled === true],
    ['waf', waf?.waf_enabled === true || waf?.rate_limit_enabled === true],
    ['errorPages', (errorPages?.pages.length ?? 0) > 0],
    ['hidden', search?.hidden === true],
    ['ownCert', cert?.enabled === true],
  ]
  return chips.filter(([, on]) => on).map(([key]) => key)
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h4 className="text-xs font-semibold tracking-wide text-muted-foreground">
        {title}
      </h4>
      <div className="space-y-2">{children}</div>
    </section>
  )
}

// Contextual next steps: only links to places that already exist.
function NextLinks({ appName, domain }: { appName: string; domain: string }) {
  const { t } = useTranslation('domains')
  const { data: network } = useAppNetwork(appName)
  const [diagnoseOpen, setDiagnoseOpen] = useState(false)
  const linkClass = cn(buttonVariants({ variant: 'ghost', size: 'sm' }))
  return (
    <div className="flex flex-wrap items-center gap-x-1 gap-y-0.5">
      <Link
        to="/domains/$domain"
        params={{ domain }}
        search={{ app: appName }}
        className={linkClass}
      >
        {t('moreOptions.links.allSettings')}
      </Link>
      <button
        type="button"
        className={linkClass}
        onClick={() => {
          setDiagnoseOpen(true)
        }}
      >
        <StethoscopeGlassIcon size="1.4em" aria-hidden="true" />
        {t('moreOptions.links.diagnose')}
      </button>
      {network && !network.running ? (
        <Link
          to="/apps/$name/deploys"
          params={{ name: appName }}
          className={linkClass}
        >
          <RocketGlassIcon size="1.4em" aria-hidden="true" />
          {t('moreOptions.links.deploys')}
        </Link>
      ) : null}
      {diagnoseOpen ? (
        <DomainDiagnoseDialog
          domain={domain}
          onClose={() => {
            setDiagnoseOpen(false)
          }}
        />
      ) : null}
    </div>
  )
}

// One collapsed disclosure for everything beyond "is it working": the
// header chips show which options are on so state stays visible closed.
export function DomainMoreOptions({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const [open, setOpen] = useState(false)
  const panelId = useId()
  const active = useActiveChips(appName, domain)
  return (
    <div className="rounded-md border border-border">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 px-2 py-1">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => {
            setOpen((v) => !v)
          }}
          className="flex items-center gap-1 rounded-sm py-0.5 text-sm font-medium focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          <ChevronDownGlassIcon
            size="1.2em"
            aria-hidden="true"
            className={cn(
              'transition-transform motion-reduce:transition-none',
              open ? 'rotate-0' : '-rotate-90',
            )}
          />
          {t('moreOptions.title')}
        </button>
        <ul
          className="flex flex-wrap items-center gap-1"
          aria-label={t('moreOptions.activeLabel')}
        >
          {active.map((key) => (
            <li key={key}>
              <Badge variant="info">{t(`moreOptions.chip.${key}`)}</Badge>
            </li>
          ))}
        </ul>
      </div>
      <div id={panelId} hidden={!open}>
        {open ? (
          <div className="space-y-4 border-t border-border p-2">
            <Group title={t('moreOptions.access')}>
              <DomainBasicAuthControl appName={appName} domain={domain} />
              <DomainAuthGateControl appName={appName} domain={domain} />
            </Group>
            <Group title={t('moreOptions.routing')}>
              <DomainDnsRecordsControl appName={appName} domain={domain} />
              <DomainRedirectControl appName={appName} domain={domain} />
              <DomainMaintenanceControl appName={appName} domain={domain} />
            </Group>
            <Group title={t('moreOptions.protection')}>
              <DomainWafControl appName={appName} domain={domain} />
              <DomainErrorPagesControl appName={appName} domain={domain} />
              <DomainSearchVisibilityControl
                appName={appName}
                domain={domain}
              />
              <DomainTLSCertControl appName={appName} domain={domain} />
            </Group>
          </div>
        ) : null}
      </div>
      <div className="border-t border-border px-1 py-0.5">
        <NextLinks appName={appName} domain={domain} />
      </div>
    </div>
  )
}
