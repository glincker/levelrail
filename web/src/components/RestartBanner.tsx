import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  applyProbe,
  backoffDelay,
  beginRestart,
  clearRestart,
  useRestartState,
} from '../lib/restartTracker'
import { fetchSystemRestart } from '../queries/systemRestart'

const PROBE_TIMEOUT_MS = 3_000
const BACK_LINGER_MS = 4_000

// Shows the persisted restart state on every page and polls until a new
// process (new boot id) answers. State lives in sessionStorage, so a
// reload mid-restart resumes the same banner instead of losing it.
export function RestartBanner() {
  const { t } = useTranslation('common')
  const queryClient = useQueryClient()
  const state = useRestartState()
  const phase = state?.phase

  useEffect(() => {
    if (phase !== 'restarting') return
    const ac = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    let attempt = 0
    const tick = async () => {
      const probeAbort = new AbortController()
      const kill = setTimeout(() => probeAbort.abort(), PROBE_TIMEOUT_MS)
      try {
        const info = await fetchSystemRestart(probeAbort.signal)
        applyProbe({ ok: true, bootId: info.boot_id })
      } catch {
        if (!ac.signal.aborted) applyProbe({ ok: false })
      } finally {
        clearTimeout(kill)
      }
      if (ac.signal.aborted) return
      attempt += 1
      timer = setTimeout(() => void tick(), backoffDelay(attempt))
    }
    void tick()
    return () => {
      ac.abort()
      if (timer) clearTimeout(timer)
    }
  }, [phase])

  useEffect(() => {
    if (phase !== 'back') return
    void queryClient.invalidateQueries()
    const id = setTimeout(clearRestart, BACK_LINGER_MS)
    return () => clearTimeout(id)
  }, [phase, queryClient])

  if (!state) return null

  if (state.phase === 'back') {
    return (
      <Alert role="status" className="mx-4 mt-2 md:mx-6">
        <AlertDescription className="flex items-center gap-2">
          <CircleCheckGlassIcon
            className="size-4.5 text-emerald-600"
            aria-hidden="true"
          />
          {t('controlPlaneRestart.back')}
        </AlertDescription>
      </Alert>
    )
  }

  if (state.phase === 'timeout') {
    return (
      <Alert variant="destructive" role="alert" className="mx-4 mt-2 md:mx-6">
        <AlertDescription className="flex flex-wrap items-center gap-2">
          <span>{t('controlPlaneRestart.timeout')}</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => beginRestart(state.oldBootId)}
          >
            {t('controlPlaneRestart.keepWaiting')}
          </Button>
          <Button size="sm" variant="ghost" onClick={clearRestart}>
            {t('controlPlaneRestart.dismiss')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <Alert role="status" aria-live="polite" className="mx-4 mt-2 md:mx-6">
      <AlertDescription className="flex items-center gap-2">
        <RefreshCwGlassIcon
          className="size-4.5 motion-safe:animate-spin"
          aria-hidden="true"
        />
        {state.sawDown
          ? t('controlPlaneRestart.waiting')
          : t('controlPlaneRestart.stopping')}
      </AlertDescription>
    </Alert>
  )
}
