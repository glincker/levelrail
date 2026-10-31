import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import type { Plugin } from '../../queries/plugins'

// Only shown when a plugin declares hosts the runtime does not yet enforce.
export function EgressBanner({
  plugins,
}: Readonly<{ plugins: readonly Plugin[] }>) {
  const { t } = useTranslation('plugins')
  const exposed = plugins.some(
    (p) => p.outbound_hosts.length > 0 && p.egress_mode !== 'proxy',
  )
  if (!exposed) return null
  return (
    <Alert>
      <AlertTitle>{t('egress.openTitle')}</AlertTitle>
      <AlertDescription>{t('egress.openBody')}</AlertDescription>
    </Alert>
  )
}
