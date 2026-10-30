import { useTranslation } from 'react-i18next'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { PageBanner } from '../PageBanner'
import { NetworkSharesDiagram } from '../banners/NetworkSharesDiagram'
import { CreateNetworkShareDialog } from '../CreateNetworkShareDialog'
import { NetworkShareTable } from '../NetworkShareTable'
import type { NetworkShare } from '../../types/networkShare'

export function NetworkSharesPage({
  shares,
}: Readonly<{ shares: NetworkShare[] }>) {
  const { t } = useTranslation('settings')
  const isEmpty = shares.length === 0
  const size = isEmpty ? 'hero' : 'compact'
  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<ServerGlassIcon size="1.4em" />}
        titleKey="settings:networkSharesPage.title"
        descriptionKey={
          isEmpty
            ? 'settings:networkSharesPage.banner.description'
            : 'settings:networkSharesPage.description'
        }
        action={<CreateNetworkShareDialog />}
        illustration={<NetworkSharesDiagram size={size} />}
        size={size}
      />
      {isEmpty ? (
        <p className="max-w-prose text-sm text-muted-foreground">
          {t('networkSharesPage.banner.whatFor')}
        </p>
      ) : (
        <NetworkShareTable shares={shares} />
      )}
    </div>
  )
}
