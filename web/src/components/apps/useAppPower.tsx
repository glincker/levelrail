import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { useRestartApp, useStartApp, useStopApp } from '../../queries/apps'
import type { AppDetail } from '../../types/appDetail'
import { AppPowerConfirm } from './AppPowerConfirm'
import { readSkipRestartConfirm, type PowerKind } from './appPower'

export interface AppPower {
  /** Opens the confirm, or restarts straight away when muted in this browser. */
  restart: () => void
  /** Stop always confirms; Start runs at once. */
  toggleRunning: () => void
  /** Opens the stop or restart confirm regardless of the opt-out. */
  ask: (kind: PowerKind) => void
  restarting: boolean
  togglingRunning: boolean
  confirmDialog: ReactNode
}

export function useAppPower(app: AppDetail): AppPower {
  const { t } = useTranslation('common')
  const restartApp = useRestartApp()
  const stopApp = useStopApp()
  const startApp = useStartApp()
  const [asking, setAsking] = useState<PowerKind | null>(null)
  const name = app.name

  const fail = (error: Error) => {
    toast.add({ title: error.message, type: 'error' })
  }

  const runRestart = () => {
    restartApp.mutate(name, {
      onSuccess: () => {
        toast.add({ title: t('appActions.restarting'), type: 'success' })
        setAsking(null)
      },
      onError: (error) => {
        fail(error)
        setAsking(null)
      },
    })
  }
  const runStop = () => {
    stopApp.mutate(name, {
      onSuccess: () => {
        toast.add({
          title: `${t('appActions.stop')}: ${name}`,
          type: 'success',
        })
        setAsking(null)
      },
      onError: (error) => {
        fail(error)
        setAsking(null)
      },
    })
  }

  const restart = () => {
    if (readSkipRestartConfirm()) runRestart()
    else setAsking('restart')
  }

  return {
    restart,
    ask: setAsking,
    toggleRunning: () => {
      if (!app.suspended) {
        setAsking('stop')
        return
      }
      startApp.mutate(name, {
        onSuccess: () => {
          toast.add({
            title: t('appPower.starting', { name }),
            type: 'success',
          })
        },
        onError: fail,
      })
    },
    restarting: restartApp.isPending,
    togglingRunning: stopApp.isPending || startApp.isPending,
    confirmDialog: (
      <AppPowerConfirm
        kind={asking}
        name={name}
        pending={restartApp.isPending || stopApp.isPending}
        onCancel={() => {
          setAsking(null)
        }}
        onConfirm={asking === 'stop' ? runStop : runRestart}
      />
    ),
  }
}
