import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  CopyGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useServerAddress } from '../../queries/serverAddress'
import { useParkedDomains } from '../../queries/parkedDomains'
import { useProxyIntegration } from '../../queries/proxyIntegration'
import { usePlatformStatus } from '../platform/usePlatformStatus'
import { ProxySetupCard } from '../ProxySetupCard'
import { SetupStepHint } from '../getStarted/SetupStepHint'
import { ReachabilityPill } from './ReachabilityPill'
import { ServerAddressCard } from './ServerAddressCard'

// One bar for HTTPS, address, reachability and parked count, with the one
// primary action on the right. Setup hint and address details sit in the disclosure.
export function DomainsStatusBar({ onAdd }: { onAdd: () => void }) {
  const { t } = useTranslation('domains')
  const { httpsOff } = usePlatformStatus()
  const { data: address } = useServerAddress()
  const { data: proxy } = useProxyIntegration()
  const { data: parked = [] } = useParkedDomains()
  const { copied, copy } = useCopyToClipboard()
  const [open, setOpen] = useState(false)

  const ip = address?.ipv4 ?? address?.ipv6
  const addressProblem = Boolean(
    address && (!ip || address.behind_nat || address.tls_terminated_upstream),
  )
  const proxyDetected = Boolean(proxy && proxy.detected.kind !== 'none')

  return (
    <div className="space-y-3">
      <Collapsible open={open} onOpenChange={setOpen}>
        <div
          role="region"
          aria-label={t('statusBar.label')}
          className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-card px-4 py-2 text-sm"
        >
          {httpsOff === undefined ? null : (
            <Badge variant={httpsOff ? 'warning' : 'success'}>
              {httpsOff ? t('statusBar.httpsOff') : t('statusBar.httpsOn')}
            </Badge>
          )}
          <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
            {t('statusBar.address')}
            {ip ? (
              <>
                <span className="truncate font-mono text-foreground">{ip}</span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label={
                    copied
                      ? t('serverAddress.copied')
                      : t('serverAddress.copy', { value: ip })
                  }
                  onClick={() => {
                    copy(ip)
                  }}
                >
                  {copied ? (
                    <CheckGlassIcon size="1.4em" />
                  ) : (
                    <CopyGlassIcon size="1.4em" />
                  )}
                </Button>
              </>
            ) : (
              <span>{t('serverAddress.none')}</span>
            )}
          </span>
          {httpsOff === false && addressProblem ? (
            <Button
              type="button"
              size="xs"
              variant="outline"
              onClick={() => {
                setOpen(true)
              }}
            >
              {t('statusBar.fixAddress')}
            </Button>
          ) : null}
          <ReachabilityPill />
          {parked.length > 0 ? (
            <a
              href="#parked-domains-heading"
              className="rounded-md hover:opacity-80"
            >
              <Badge variant="muted">
                {t('statusBar.parked', { count: parked.length })}
              </Badge>
            </a>
          ) : null}
          <div className="ml-auto flex items-center gap-3">
            <CollapsibleTrigger className="flex w-auto items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
              {t('statusBar.details')}
              <ChevronDownGlassIcon className="size-4.5" aria-hidden="true" />
            </CollapsibleTrigger>
            {httpsOff ? (
              <Link
                to="/settings/platform/https"
                className={cn(buttonVariants({ size: 'sm' }))}
              >
                {t('statusBar.enableHttps')}
              </Link>
            ) : (
              <Button type="button" size="sm" onClick={onAdd}>
                <PlusGlassIcon size="1.4em" />
                {t('page.add')}
              </Button>
            )}
          </div>
        </div>
        <CollapsiblePanel className="space-y-3 pt-3">
          <SetupStepHint step="app_domain" />
          <ServerAddressCard />
        </CollapsiblePanel>
      </Collapsible>
      {proxyDetected ? <ProxySetupCard /> : null}
    </div>
  )
}
