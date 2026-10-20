import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import type { IngressSettings } from '../queries/domains'
import { useUpdateIngressSettings } from '../queries/domains'

// Shown when real certificates are on but cannot work from this ingress:
// the ports are not 80 and 443 and no DNS-01 provider is configured.
export function AcmeBlockedNotice({ settings }: { settings: IngressSettings }) {
  const { t } = useTranslation('domains')
  const update = useUpdateIngressSettings()
  if (!settings.acme_blocked) return null
  return (
    <Alert variant="destructive" className="mb-4">
      <WarningIcon />
      <AlertTitle>{t('acmeBlocked.title')}</AlertTitle>
      <AlertDescription className="space-y-3">
        <p>
          {t('acmeBlocked.body', {
            http: settings.ingress_http_port ?? 8088,
            https: settings.ingress_https_port ?? 8443,
          })}
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={update.isPending}
          onClick={() => {
            update.mutate({
              primary_domain: settings.primary_domain ?? '',
              acme_enabled: false,
              acme_email: settings.acme_email ?? '',
              acme_directory_url: settings.acme_directory_url ?? '',
              hsts_enabled: settings.hsts_enabled,
            })
          }}
        >
          {update.isPending
            ? t('acmeBlocked.turningOff')
            : t('acmeBlocked.turnOff')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
