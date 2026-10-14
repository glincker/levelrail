import { useContext, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useIsDemo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import { installDemoFetchGuard } from '@/lib/demoFetchGuard'
import { isTourDismissed } from '@/lib/demoTour'
import { DemoBanner } from './DemoBanner'
import { DemoTour } from './DemoTour'

const TOAST_COOLDOWN_MS = 3000

/** DemoChrome mounts the demo banner, tour and blocked-write toast; it renders nothing outside demo mode. */
export function DemoChrome() {
  const isDemo = useIsDemo()
  const { t } = useTranslation('demo')
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || ''
  const [tourOpen, setTourOpen] = useState(() => !isTourDismissed())

  useEffect(() => {
    if (!isDemo) return
    let last = 0
    installDemoFetchGuard(() => {
      const now = Date.now()
      if (now - last < TOAST_COOLDOWN_MS) return
      last = now
      toast.add({
        title: t('readOnly.toastTitle'),
        description: t('readOnly.toastBody', { brand: brandName }),
        type: 'info',
      })
    })
  }, [isDemo, t, brandName])

  if (!isDemo) return null
  return (
    <>
      <DemoBanner
        onTour={() => {
          setTourOpen(true)
        }}
      />
      {tourOpen ? (
        <DemoTour
          onClose={() => {
            setTourOpen(false)
          }}
        />
      ) : null}
    </>
  )
}
