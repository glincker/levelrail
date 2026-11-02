import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { InfoGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { FREE_PROVIDER_HOST } from '../../lib/freeDnsHost'
import { httpsStatusQueryOptions } from '../../queries/httpsStatus'
import { useAppNetwork } from '../../queries/appNetwork'

/** AutoUrlInfo folds the automatic-address notes into one popover beside the URL. */
export function AutoUrlInfo({ appName }: { appName: string }) {
  const { t } = useTranslation(['networkProxy', 'common'])
  const { data: network } = useAppNetwork(appName)
  const { data: https } = useQuery(httpsStatusQueryOptions())
  const url = network?.fallback_url
  if (!url) return null
  const beta = FREE_PROVIDER_HOST.test(url)
  const selfSigned =
    url.startsWith('https://') &&
    https !== undefined &&
    https.state !== 'issued'
  if (!beta && !selfSigned) return null
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="ghost" size="icon-xs" />}
        aria-label={t('common:overviewHero.addressInfo')}
      >
        <InfoGlassIcon className="size-4.5" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 gap-2 text-xs">
        {beta ? <p>{t('networkProxy:automaticDomain.betaHint')}</p> : null}
        {selfSigned ? <p>{t('networkProxy:automaticDomain.tlsNote')}</p> : null}
      </PopoverContent>
    </Popover>
  )
}
