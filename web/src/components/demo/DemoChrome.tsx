import { useContext, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useRouterState } from '@tanstack/react-router'
import { toast } from '@/components/ui/toast'
import { useDemoInfo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import {
  apiRoutePattern,
  captureDemoEvent,
  initDemoAnalytics,
} from '@/lib/demoAnalytics'
import { installDemoFetchGuard } from '@/lib/demoFetchGuard'
import { isTourDismissed } from '@/lib/demoTour'
import { DemoBanner } from './DemoBanner'
import { DemoTour } from './DemoTour'

const TOAST_COOLDOWN_MS = 3000

/** DemoChrome mounts the demo banner, tour and blocked-write toast; it renders nothing outside demo mode. */
export function DemoChrome() {
  const info = useDemoInfo()
  const isDemo = info.enabled
  const routeId = useRouterState({ select: (s) => s.matches.at(-1)?.routeId })
  const { t } = useTranslation('demo')
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || ''
  const [tourOpen, setTourOpen] = useState(() => !isTourDismissed())

  useEffect(() => {
    if (isDemo) void initDemoAnalytics(info)
  }, [isDemo, info])

  useEffect(() => {
    if (isDemo && routeId)
      captureDemoEvent('demo_route_viewed', { route: routeId })
  }, [isDemo, routeId])

  useEffect(() => {
    if (!isDemo) return
    let last = 0
    installDemoFetchGuard((b) => {
      if (b.code === 'demo_readonly') {
        captureDemoEvent('demo_readonly_blocked', {
          method: b.method,
          route: apiRoutePattern(b.path),
        })
      }
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
