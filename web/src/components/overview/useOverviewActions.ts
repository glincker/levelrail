import { useNavigate } from '@tanstack/react-router'
import { toast } from '@/components/ui/toast'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { useRestartApp, useStartApp, useStopApp } from '../../queries/apps'
import type { AppDetail } from '../../types/appDetail'

/** The app-level actions shared by the hero buttons, the palette and hotkeys. */
export function useOverviewActions(app: AppDetail, url: string | null) {
  const navigate = useNavigate()
  const restart = useRestartApp()
  const stop = useStopApp()
  const start = useStartApp()
  const { redeploy, isPending: redeploying } = useRedeployApp(
    app.name,
    app.image,
  )
  const { copied, copy } = useCopyToClipboard()

  const notify = (title: string) => () => toast.add({ title, type: 'success' })
  const fail = (error: Error) =>
    toast.add({ title: error.message, type: 'error' })

  return {
    redeploy,
    redeploying,
    restart: () =>
      restart.mutate(app.name, {
        onSuccess: notify(`Restarting "${app.name}".`),
        onError: fail,
      }),
    restarting: restart.isPending,
    toggleRunning: () =>
      app.suspended
        ? start.mutate(app.name, {
            onSuccess: notify(`Starting "${app.name}".`),
            onError: fail,
          })
        : stop.mutate(app.name, {
            onSuccess: notify(`Stopping "${app.name}".`),
            onError: fail,
          }),
    togglingRunning: stop.isPending || start.isPending,
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
