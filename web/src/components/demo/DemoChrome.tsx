import { useContext, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useRouterState } from '@tanstack/react-router'
import { toast, toastAction } from '@/components/ui/toast'
import { useDemoInfo } from '@/hooks/useDemo'
import { BrandContext } from '@/lib/brandContext'
import {
  apiRoutePattern,
  captureDemoEvent,
  initDemoAnalytics,
} from '@/lib/demoAnalytics'
import { installDemoFetchGuard, type DemoBlock } from '@/lib/demoFetchGuard'
import {
  createDemoToastGate,
  installUserActionTracking,
  showDemoToast,
} from '@/lib/demoToast'
import { DEMO_INSTALL_EVENT } from '@/lib/installPrompt'
import { isTourDismissed } from '@/lib/demoTour'
import { DemoBanner } from './DemoBanner'
import { DemoInstallDialog } from './DemoInstallDialog'
import { DemoTour } from './DemoTour'

/** DemoChrome mounts the demo bar, install dialog, tour and the single read-only notice; it renders nothing outside demo mode. */
export function DemoChrome() {
  const info = useDemoInfo()
  const isDemo = info.enabled
  const routeId = useRouterState({ select: (s) => s.matches.at(-1)?.routeId })
  const { t } = useTranslation('demo')
  const brand = useContext(BrandContext)
  const brandName = brand?.ShortName || brand?.Name || ''
  const [tourDismissed, setTourDismissed] = useState(() => isTourDismissed())
  const [installOpen, setInstallOpen] = useState(false)
  const onBlocked = useRef<(b: DemoBlock) => void>(() => undefined)

  useEffect(() => {
    if (isDemo) void initDemoAnalytics(info)
  }, [isDemo, info])

  useEffect(() => {
    if (isDemo && routeId)
      captureDemoEvent('demo_route_viewed', { route: routeId })
  }, [isDemo, routeId])

  useEffect(() => {
    if (!isDemo) return
    const gate = createDemoToastGate()
    onBlocked.current = (b) => {
      if (b.code === 'demo_readonly') {
        captureDemoEvent('demo_readonly_blocked', {
          method: b.method,
          route: apiRoutePattern(b.path),
        })
      }
      if (b.code !== 'demo_readonly' || !gate.accept(b.method)) return
      showDemoToast(toast, {
        title: t('readOnly.toast', { brand: brandName }),
        type: 'info',
        actionProps: toastAction(t('readOnly.installAction'), () => {
          setInstallOpen(true)
        }),
      })
    }
    const stop = installUserActionTracking(gate)
    installDemoFetchGuard((b) => {
      onBlocked.current(b)
    })
    return stop
  }, [isDemo, t, brandName])

  // Claims the event so openDemoInstall() callers do not fall back to the docs tab.
  useEffect(() => {
    if (!isDemo) return
    const onInstall = (e: Event) => {
      e.preventDefault()
      setInstallOpen(true)
    }
    window.addEventListener(DEMO_INSTALL_EVENT, onInstall)
    return () => {
      window.removeEventListener(DEMO_INSTALL_EVENT, onInstall)
    }
  }, [isDemo])

  if (!isDemo) return null
  return (
    <>
      <DemoBanner
        onInstall={() => {
          setInstallOpen(true)
        }}
      />
      <DemoInstallDialog open={installOpen} onOpenChange={setInstallOpen} />
      {tourDismissed ? null : (
        <DemoTour
          onDismiss={() => {
            setTourDismissed(true)
          }}
          onInstall={() => {
            setInstallOpen(true)
          }}
        />
      )}
    </>
  )
}
