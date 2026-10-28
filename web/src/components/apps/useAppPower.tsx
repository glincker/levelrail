import { useEffect, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast, toastAction } from '@/components/ui/toast'
import { useRestartApp, useStartApp, useStopApp } from '../../queries/apps'
import { deployStatusQueryOptions } from '../../queries/deploys'
import type { AppDetail } from '../../types/appDetail'
import { AppPowerConfirm } from './AppPowerConfirm'
import {
  POWER_TIMING,
  beginPowerTransition,
  endPowerTransition,
  evaluatePower,
  getPowerTransition,
  readSkipRestartConfirm,
  updatePowerTransition,
  usePowerTransition,
  type PowerKind,
} from './appPower'

export interface AppPower {
  /** Opens the confirm, or restarts straight away when muted in this browser. */
  restart: () => void
  /** Stop always confirms; Start runs at once. */
  toggleRunning: () => void
  /** Opens the stop or restart confirm regardless of the opt-out. */
  ask: (kind: PowerKind) => void
  /** True from the click until the app is back, so no double submit. */
  restarting: boolean
  stopping: boolean
  /** The wait passed the timeout; the button is usable again. */
  slowRestart: boolean
  restartLabel: string
  togglingRunning: boolean
  confirmDialog: ReactNode
}

export function useAppPower(app: AppDetail): AppPower {
  const { t } = useTranslation('common')
  const navigate = useNavigate()
  const restartApp = useRestartApp()
  const stopApp = useStopApp()
  const startApp = useStartApp()
  const [asking, setAsking] = useState<PowerKind | null>(null)
  const name = app.name
  const transition = usePowerTransition(name)
  const active = transition !== undefined
  const { data: conditions, dataUpdatedAt } = useQuery({
    ...deployStatusQueryOptions(name),
    refetchInterval: POWER_TIMING.pollMs,
    enabled: active,
  })
  const [tick, setTick] = useState(0)

  useEffect(() => {
    if (!active) return
    const id = setInterval(() => {
      setTick((n) => n + 1)
    }, 1000)
    return () => {
      clearInterval(id)
    }
  }, [active])

  useEffect(() => {
    const cur = getPowerTransition(name)
    if (!cur || !conditions) return
    const logs = toastAction(t('appPower.viewLogs'), () => {
      void navigate({ to: '/apps/$name/logs', params: { name } })
    })
    const at = Date.now()
    const out = evaluatePower(cur, conditions, at, POWER_TIMING)
    if (out.state === 'done') {
      if (endPowerTransition(name)) {
        toast.add({
          title: t(
            cur.kind === 'stop' ? 'appPower.stopped' : 'appPower.restarted',
            { name },
          ),
          type: 'success',
        })
      }
    } else if (
      out.state === 'failed' &&
      cur.failStreak + (dataUpdatedAt === cur.lastPollAt ? 0 : 1) >=
        POWER_TIMING.failPolls
    ) {
      if (endPowerTransition(name)) {
        toast.add({
          title: t('appPower.restartFailed', { name }),
          description: out.reason,
          type: 'error',
          actionProps: logs,
        })
      }
    } else {
      if (dataUpdatedAt !== cur.lastPollAt) {
        updatePowerTransition(name, {
          lastPollAt: dataUpdatedAt,
          failStreak: out.state === 'failed' ? cur.failStreak + 1 : 0,
        })
      }
      if (out.state === 'pending' && out.busy && !cur.seenBusy) {
        updatePowerTransition(name, { seenBusy: true })
      }
      if (!cur.slow && at - cur.startedAt >= POWER_TIMING.timeoutMs) {
        updatePowerTransition(name, { slow: true })
        toast.add({
          title: t(
            cur.kind === 'stop'
              ? 'appPower.stillStoppingToast'
              : 'appPower.stillRestartingToast',
            { name },
          ),
          description: t('appPower.stillRestartingHint'),
          type: 'warning',
          actionProps: logs,
        })
      }
    }
  }, [conditions, dataUpdatedAt, tick, name, t, navigate])

  const fail = (error: Error) => {
    toast.add({ title: error.message, type: 'error' })
  }
  const run = (kind: PowerKind) => {
    const mutation = kind === 'stop' ? stopApp : restartApp
    mutation.mutate(name, {
      onSuccess: () => {
        beginPowerTransition(name, kind)
        setAsking(null)
      },
      onError: (error) => {
        fail(error)
        setAsking(null)
      },
    })
  }

  const restart = () => {
    if (readSkipRestartConfirm()) run('restart')
    else setAsking('restart')
  }

  const restartRunning =
    restartApp.isPending || (transition?.kind === 'restart' && !transition.slow)
  const slowRestart = transition?.kind === 'restart' && transition.slow
  const stopping =
    stopApp.isPending || (transition?.kind === 'stop' && !transition.slow)

  let restartLabel = t('appActions.restart')
  if (restartRunning) restartLabel = t('appPower.restarting')
  else if (slowRestart) restartLabel = t('appPower.stillRestarting')

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
    restarting: restartRunning,
    stopping,
    slowRestart,
    restartLabel,
    togglingRunning: stopping || startApp.isPending,
    confirmDialog: (
      <AppPowerConfirm
        kind={asking}
        name={name}
        pending={restartApp.isPending || stopApp.isPending}
        onCancel={() => {
          setAsking(null)
        }}
        onConfirm={() => {
          if (asking) run(asking)
        }}
      />
    ),
  }
}
