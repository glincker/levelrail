import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HostFirewallToggle } from './HostFirewallToggle'
import { useHostFirewall } from '../queries/hostFirewall'
import type { HostFirewallPublishedPort } from '../types/hostFirewall'

const LOOPBACK_BINDS = new Set(['127.0.0.1', '::1', 'localhost'])

function portState(
  p: HostFirewallPublishedPort,
  managed: boolean,
): {
  variant: 'success' | 'muted' | 'warning'
  label:
    | 'hostFirewall.notFiltered'
    | 'hostFirewall.notChecked'
    | 'hostFirewall.allowed'
    | 'hostFirewall.localOnly'
    | 'hostFirewall.notAllowed'
} {
  if (p.filtered === false)
    return { variant: 'warning', label: 'hostFirewall.notFiltered' }
  if (!managed) return { variant: 'muted', label: 'hostFirewall.notChecked' }
  if (p.allowed) return { variant: 'success', label: 'hostFirewall.allowed' }
  return LOOPBACK_BINDS.has(p.bind_address)
    ? { variant: 'muted', label: 'hostFirewall.localOnly' }
    : { variant: 'warning', label: 'hostFirewall.notAllowed' }
}

export function HostFirewallCard() {
  const { t } = useTranslation('settings')
  const { data } = useHostFirewall()
  if (!data) return null
  const managed = data.managed ?? data.installed
  const backend = data.backend ?? (data.installed ? 'ufw' : 'none')
  const statusKey = managed
    ? data.active
      ? 'hostFirewall.statusOn'
      : 'hostFirewall.statusOff'
    : backend === 'none'
      ? 'hostFirewall.statusNone'
      : 'hostFirewall.statusUnmanaged'

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <ShieldCheckGlassIcon
            className="size-5 text-muted-foreground"
            aria-hidden="true"
          />
          {t('hostFirewall.title')}
          <Badge variant={data.active ? 'success' : 'muted'}>
            {t(statusKey, { backend })}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p className="text-muted-foreground">
          {t(
            managed
              ? 'hostFirewall.description'
              : backend === 'none'
                ? 'hostFirewall.noneHelp'
                : 'hostFirewall.unmanagedHelp',
          )}
        </p>
        {!managed && data.detail ? (
          <p className="text-xs text-muted-foreground">{data.detail}</p>
        ) : null}
        <>
          <p className="text-xs text-muted-foreground">
            {t('hostFirewall.required')}:{' '}
            <span className="break-all font-mono">
              {data.required.map((p) => `${p.port}/${p.protocol}`).join(', ')}
            </span>
          </p>
          {data.published_ports?.length ? (
            <ul
              aria-label={t('hostFirewall.published')}
              className="space-y-1 text-xs"
            >
              {data.published_ports.map((p) => (
                <li
                  key={`${p.port}/${p.protocol}/${p.app}/${p.service ?? ''}`}
                  className="flex flex-wrap items-center gap-2"
                >
                  <span className="font-mono">{p.port}</span>
                  <Badge variant={p.protocol === 'udp' ? 'info' : 'muted'}>
                    {p.protocol.toUpperCase()}
                  </Badge>
                  <span className="text-muted-foreground">
                    {p.service ? `${p.app} / ${p.service}` : p.app}
                  </span>
                  <span className="inline-flex items-center gap-1.5">
                    <span className="font-mono text-muted-foreground">
                      {p.bind_address}
                    </span>
                    <Badge variant={portState(p, managed).variant}>
                      {t(portState(p, managed).label)}
                    </Badge>
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
          {managed ? (
            <p className="text-xs text-muted-foreground">
              {t('hostFirewall.sshNote')}
            </p>
          ) : null}
          {managed ? <HostFirewallToggle /> : null}
        </>
      </CardContent>
    </Card>
  )
}
