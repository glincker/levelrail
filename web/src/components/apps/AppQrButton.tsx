import { useTranslation } from 'react-i18next'
import { QrCodeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { UrlReachability } from '../../lib/urlReachability'
import { AppQrPanel } from './AppQrPanel'

/** Small QR icon for the URL row; hidden unless the address can be opened from a phone. */
export function AppQrButton({
  appName,
  url,
  shareUrl,
  reach,
}: {
  appName: string
  url: string
  shareUrl: string | null
  reach: UrlReachability
}) {
  const { t } = useTranslation('appOverview')
  if (!shareUrl) return null
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={t('status.qr.open')}
          />
        }
      >
        <QrCodeGlassIcon className="size-5" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <AppQrPanel
          appName={appName}
          url={url}
          shareUrl={shareUrl}
          reach={reach}
          size={144}
        />
      </PopoverContent>
    </Popover>
  )
}
