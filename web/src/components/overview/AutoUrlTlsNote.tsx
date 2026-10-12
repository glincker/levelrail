import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { useAppNetwork } from '../../queries/appNetwork'

/** AutoUrlTlsNote warns that the automatic https URL is self-signed until HTTPS is enabled. */
export function AutoUrlTlsNote({ appName }: { appName: string }) {
  const { t } = useTranslation('networkProxy')
  const { data: network } = useAppNetwork(appName)
  const { data: https } = useQuery(httpsStatusQueryOptions())
  const url = network?.fallback_url
  if (!url || !url.startsWith('https://')) return null
  if (!https || https.state === 'issued') return null
  return (
    <p className="text-xs text-muted-foreground">
      {t('automaticDomain.tlsNote')}
    </p>
  )
}
