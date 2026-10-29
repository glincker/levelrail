import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HostFirewallToggle } from './HostFirewallToggle'
import { useHostFirewall } from '../queries/hostFirewall'

const LOOPBACK_BINDS = new Set(['127.0.0.1', '::1', 'localhost'])

export function HostFirewallCard() {
  const { t } = useTranslation('settings')
  const { data } = useHostFirewall()
  if (!data) return null

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
            {t(
              !data.installed
                ? 'hostFirewall.statusMissing'
                : data.active
                  ? 'hostFirewall.statusOn'
                  : 'hostFirewall.statusOff',
            )}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p className="text-muted-foreground">
          {t(
            data.installed
              ? 'hostFirewall.description'
              : 'hostFirewall.missingHelp',
          )}
        </p>
        {data.installed ? (
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
                      <Badge
                        variant={
                          p.allowed
                            ? 'success'
                            : LOOPBACK_BINDS.has(p.bind_address)
                              ? 'muted'
                              : 'warning'
                        }
                      >
                        {t(
                          p.allowed
                            ? 'hostFirewall.allowed'
                            : LOOPBACK_BINDS.has(p.bind_address)
                              ? 'hostFirewall.localOnly'
                              : 'hostFirewall.notAllowed',
                        )}
                      </Badge>
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
            <p className="text-xs text-muted-foreground">
              {t('hostFirewall.sshNote')}
            </p>
            <HostFirewallToggle />
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
