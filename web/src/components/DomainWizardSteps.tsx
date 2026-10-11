import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
import { certificatePath, suggestPort } from '../lib/domainWizard'
import type { DomainCheckResult } from '../queries/domainCheck'
import type {
  IngressConnectivity,
  ListeningPorts,
} from '../queries/domainWizard'
import { DnsProviderBadge } from './DnsProviderBadge'

export function WizardStep({
  index,
  title,
  children,
}: {
  index: number
  title: string
  children: ReactNode
}) {
  return (
    <section className="space-y-2 rounded-md border border-border p-3">
      <h4 className="flex items-center gap-2 text-sm font-medium text-foreground">
        <span
          className="flex size-5 items-center justify-center rounded-full bg-muted text-xs text-muted-foreground"
          aria-hidden="true"
        >
          {index}
        </span>
        {title}
      </h4>
      {children}
    </section>
  )
}

// What the probe saw inside the running container, with the one fix on offer.
export function PortStepBody({
  appPort,
  data,
  isFetching,
  switching,
  onSwitch,
  onRecheck,
}: {
  appPort: number
  data?: ListeningPorts
  isFetching: boolean
  switching: boolean
  onSwitch: (port: number) => void
  onRecheck: () => void
}) {
  const { t } = useTranslation('domains')
  const suggestion = suggestPort(appPort, data?.listening ?? [])
  let message = t('port.unknown', { port: appPort })
  let ok = false
  if (data?.verdict === 'listening') {
    message = t('port.listening', { port: appPort })
    ok = true
  } else if (data?.verdict === 'other_port') {
    message = t('port.otherPort', {
      port: appPort,
      ports: data.listening.join(', '),
    })
  } else if (data?.verdict === 'not_listening') {
    message = t('port.notListening', { port: appPort })
  } else if (data?.reason === 'exec_disabled') {
    message = `${t('port.unknown', { port: appPort })} ${t('port.execDisabled')}`
  } else if (data?.reason === 'not_running') {
    message = `${t('port.notRunning')} ${t('port.unknown', { port: appPort })}`
  }
  return (
    <div className="space-y-2 text-xs">
      <p className="text-muted-foreground">
        {t('port.description', { port: appPort })}
      </p>
      <p
        className={
          ok ? 'flex items-center gap-1.5 text-foreground' : 'text-foreground'
        }
      >
        {ok ? (
          <CheckCircleIcon className="size-4 shrink-0" aria-hidden="true" />
        ) : null}
        {message}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {suggestion.switchTo ? (
          <Button
            type="button"
            size="sm"
            disabled={switching}
            onClick={() => {
              onSwitch(suggestion.switchTo as number)
            }}
          >
            {t('port.useListening', { port: suggestion.switchTo })}
          </Button>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={isFetching}
          onClick={onRecheck}
        >
          {t('port.recheck')}
        </Button>
      </div>
    </div>
  )
}

// Which certificate path applies, and what to do when HTTP-01 cannot work.
export function CertificateStepBody({
  domain,
  check,
  connectivity,
}: {
  domain: string
  check?: DomainCheckResult
  connectivity?: IngressConnectivity
}) {
  const { t } = useTranslation('domains')
  const privateHost = check?.expected_private ?? connectivity?.private ?? false
  const path = certificatePath(domain, check?.challenge, privateHost)
  const provider = check?.dns_provider ?? connectivity?.dns_provider ?? 'none'
  const host = check?.expected_host ?? connectivity?.host ?? ''
  const needsDns01 = path === 'dns-01-private' || path === 'dns-01-wildcard'
  return (
    <div className="space-y-2 text-xs">
      {path === 'http-01' || path === 'upstream-proxy' ? (
        <p className="text-foreground">
          {t(
            path === 'http-01'
              ? 'certificate.http01'
              : 'certificate.upstreamProxy',
          )}
        </p>
      ) : (
        <Alert variant="destructive">
          <WarningCircleIcon aria-hidden="true" />
          <AlertDescription>
            {path === 'dns-01-private'
              ? t('certificate.dns01Private', { host })
              : t('certificate.dns01Wildcard')}
          </AlertDescription>
        </Alert>
      )}
      <DnsProviderBadge provider={provider} />
      {needsDns01 ? (
        <p className="text-foreground">
          {provider === 'none'
            ? `${t('certificate.providerNone')} ${t('certificate.setupStep')}`
            : t('certificate.providerActive', {
                provider: t(`provider.${provider}`),
              })}
        </p>
      ) : null}
      {connectivity && connectivity.ports.length > 0 ? (
        <div className="space-y-0.5">
          <p className="font-medium text-foreground">
            {t('certificate.connectivity')}
          </p>
          <ul className="text-muted-foreground">
            {connectivity.ports.map((p) => (
              <li key={p.port}>
                {p.reachable
                  ? t('certificate.portOpen', { port: p.port })
                  : t('certificate.portClosed', { port: p.port })}
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground">
            {t('certificate.connectivityNote')}
          </p>
        </div>
      ) : null}
      <p className="text-muted-foreground">{t('certificate.stagingNote')}</p>
      {needsDns01 && provider === 'none' ? (
        <Link
          to="/domains"
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          {t('certificate.openDomains')}
        </Link>
      ) : null}
    </div>
  )
}
