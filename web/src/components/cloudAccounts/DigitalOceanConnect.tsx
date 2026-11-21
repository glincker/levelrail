import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  useDigitalOceanOAuthStatus,
  useStartDigitalOceanOAuth,
} from '../../queries/cloudAccounts'
import { CloudProviderMark } from './CloudProviderMark'

// Shown only to roots. Without the OAuth app settings it explains what to set instead of a dead button.
export function DigitalOceanConnect() {
  const { t } = useTranslation('cloudAccounts')
  const status = useDigitalOceanOAuthStatus(true)
  const start = useStartDigitalOceanOAuth()

  if (status.isPending || status.isError) return null
  if (!status.data.enabled) {
    return (
      <p className="text-xs text-muted-foreground">
        {t('section.connectDigitalOceanHint')}
      </p>
    )
  }
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={start.isPending}
      onClick={() => {
        start.mutate(undefined, {
          onSuccess: (res) => {
            window.location.assign(res.url)
          },
        })
      }}
    >
      <CloudProviderMark provider="digitalocean" className="size-4" />
      <PlusGlassIcon size="1.4em" aria-hidden="true" />
      {t('section.connectDigitalOcean')}
    </Button>
  )
}
