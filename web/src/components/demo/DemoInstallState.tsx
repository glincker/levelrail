import { useContext } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { useDemoInfo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import { openDemoInstall } from '@/lib/installPrompt'

/** DemoInstallState replaces a feature the demo cannot serve with an honest install prompt. */
export function DemoInstallState() {
  const { t } = useTranslation('demo')
  const info = useDemoInfo()
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || 'Levelrail'
  return (
    <EmptyState
      icon={<LockGlassIcon className="size-5" />}
      title={t('installState.title')}
      description={t('installState.description', { brand: brandName })}
      action={
        <Button
          type="button"
          size="sm"
          onClick={() => {
            openDemoInstall(info.docsUrl)
          }}
        >
          {t('installState.install', { brand: brandName })}
        </Button>
      }
    />
  )
}
