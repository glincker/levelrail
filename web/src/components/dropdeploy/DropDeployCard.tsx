import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { SitePublisher } from './SitePublisher'

/** Overview card for a static app: drop a folder or zip to publish it with no git and no build. */
export function DropDeployCard({ appName }: { appName: string }) {
  const { t } = useTranslation('newApp')
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <UploadGlassIcon className="size-4.5" aria-hidden="true" />
          {t('drop.cardTitle')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          {t('drop.cardDescription')}
        </p>
        <SitePublisher appName={appName} />
      </CardContent>
    </Card>
  )
}
