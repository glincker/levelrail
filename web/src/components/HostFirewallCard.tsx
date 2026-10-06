import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HostFirewallToggle } from './HostFirewallToggle'
import { useHostFirewall } from '../queries/hostFirewall'

export function HostFirewallCard() {
  const { t } = useTranslation('settings')
  const { data } = useHostFirewall()
  if (!data) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-sm">
          <ShieldCheckIcon
            className="size-4 text-muted-foreground"
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
      <CardContent className="space-y-3 text-sm">
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
              <span className="font-mono">
                {data.required.map((p) => `${p.port}/${p.protocol}`).join(', ')}
              </span>
            </p>
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
