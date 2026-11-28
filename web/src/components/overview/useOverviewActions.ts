import { useNavigate } from '@tanstack/react-router'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useTranslation } from 'react-i18next'
import { toast, toastAction } from '@/components/ui/toast'
import { useAppPower } from '../apps/useAppPower'
import { useUrlReachability } from './useUrlReachability'
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
  const { t } = useTranslation('appOverview')
  const { reach } = useUrlReachability(app, url)

  function openUrl(target: string) {
    window.open(target, '_blank', 'noopener,noreferrer')
  }

  // One gate for the O key, the palette and the Open button: a dead address
  // explains itself and offers the fix instead of opening a blank tab.
  function openApp() {
    if (!url) return
    if (reach.state !== 'down' && reach.state !== 'local_only') {
      openUrl(url)
      return
    }
    if (reach.reason === 'stopped') {
      toast.add({
        title: t('status.openBlocked.stopped', { name: app.name }),
        type: 'warning',
        actionProps: toastAction(t('status.openBlocked.start'), () =>
          power.toggleRunning(),
        ),
      })
    } else if (reach.reason === 'not_running') {
      toast.add({
        title: t('status.openBlocked.not_running', { name: app.name }),
        type: 'warning',
        actionProps: toastAction(t('status.openBlocked.restart'), () =>
          power.restart(),
        ),
      })
    } else if (reach.state === 'local_only') {
      toast.add({
        title: t('status.openBlocked.local_only'),
        type: 'info',
        actionProps: toastAction(t('status.openAnyway'), () => openUrl(url)),
      })
    } else {
      toast.add({
        title: t('status.openBlocked.not_ready', { name: app.name }),
        type: 'warning',
      })
    }
  }

  return {
    redeploy,
    redeploying,
    restart: power.restart,
    restarting: power.restarting,
    restartLabel: power.restartLabel,
    stopping: power.stopping,
    slowRestart: power.slowRestart,
    toggleRunning: power.toggleRunning,
    togglingRunning: power.togglingRunning,
    confirmDialog: power.confirmDialog,
    openApp,
    copyUrl: () => {
      if (url) copy(url)
    },
    copied,
    goToDeploys: () =>
      void navigate({ to: '/apps/$name/deploys', params: { name: app.name } }),
  }
}
