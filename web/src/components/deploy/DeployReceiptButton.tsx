import { useTranslation } from 'react-i18next'
import { DownloadGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { deployReceiptUrl } from './deployReceiptUrl'

export function DeployReceiptButton({
  appName,
  deployId,
}: {
  appName: string
  deployId: string
}) {
  const { t } = useTranslation('deployDetail')
  return (
    <Button
      variant="outline"
      size="sm"
      nativeButton={false}
      render={
        <a
          href={deployReceiptUrl(appName, deployId)}
          download={`${deployId}.receipt.json`}
          title={t('receipt.hint')}
        />
      }
    >
      <DownloadGlassIcon className="size-5" data-icon="inline-start" />
      {t('receipt.button')}
    </Button>
  )
}
