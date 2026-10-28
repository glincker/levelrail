import { useNavigate } from '@tanstack/react-router'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useAppPower } from '../apps/useAppPower'
import type { AppDetail } from '../../types/appDetail'

/** The app-level actions shared by the hero buttons, the palette and hotkeys. */
export function useOverviewActions(app: AppDetail, url: string | null) {
  const navigate = useNavigate()
  const power = useAppPower(app)
  const { redeploy, isPending: redeploying } = useRedeployApp(
    app.name,
    app.image,
  )
  const { copied, copy } = useCopyToClipboard()

  return {
    redeploy,
    redeploying,
    restart: power.restart,
    restarting: power.restarting,
    toggleRunning: power.toggleRunning,
    togglingRunning: power.togglingRunning,
    confirmDialog: power.confirmDialog,
    openApp: () => {
      if (url) window.open(url, '_blank', 'noopener,noreferrer')
    },
    copyUrl: () => {
      if (url) copy(url)
    },
    copied,
    goToDeploys: () =>
      void navigate({ to: '/apps/$name/deploys', params: { name: app.name } }),
  }
}
