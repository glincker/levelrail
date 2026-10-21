import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import {
  preflightQueryOptions,
  updatesQueryOptions,
  type UpdateStatus,
} from '@/queries/updates'
import {
  selfUpgradeAttemptsQueryOptions,
  type SelfUpgradeAttempt,
} from '@/queries/selfUpgrade'
import { scheduleQueryOptions } from '@/queries/upgradeSchedule'
import { formatRelative } from '@/components/kit/formatRelative'
import { UpgradeNow } from '../UpgradeNow'
import { UpdateChanges } from './UpdateChanges'
import { deriveUpdateState } from './updateState'
import { UpdateStatusView } from './UpdateStatusView'
import { UpgradeStepper } from './UpgradeStepper'
import { deriveStepper } from '@/lib/upgradeStepper'

const RECENT_MS = 15 * 60_000

function recentFinished(
  list: SelfUpgradeAttempt[],
): SelfUpgradeAttempt | undefined {
  const last = list
    .filter((a) => a.outcome !== 'running')
    .sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))[0]
  return last && Date.now() - Date.parse(last.started_at) < RECENT_MS
    ? last
    : undefined
}

// UpdateStatusHeader owns the page's top card: derived state, the manual
// re-check, the change preview, and the existing upgrade controls.
export function UpdateStatusHeader({ initial }: { initial: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const [checking, setChecking] = useState(false)
  const query = useQuery({ ...updatesQueryOptions(), initialData: initial })
  const preflight = useQuery(preflightQueryOptions())
  const schedule = useQuery(scheduleQueryOptions())
  const attempts = useQuery(selfUpgradeAttemptsQueryOptions(false))

  const status = query.data
  const edge = status.release_kind === 'edge_image'
  const state = deriveUpdateState({
    latestVersion: status.latest_version,
    updateAvailable: status.update_available,
    hostBinaryEdge: edge && preflight.data?.edge?.installable === false,
    scheduleActive:
      schedule.data?.active?.state === 'pending' ||
      schedule.data?.active?.state === 'blocked' ||
      schedule.data?.active?.state === 'running',
    upgradeRunning: (attempts.data ?? []).some((a) => a.outcome === 'running'),
    checkFailed: query.isRefetchError,
  })

  const list = attempts.data ?? []
  const running = list.find((a) => a.outcome === 'running')
  const recentDone = recentFinished(list)
  const steps = deriveStepper({
    updateAvailable: status.update_available,
    checkFailed: query.isRefetchError,
    preflightBlocked: preflight.data?.blocked === true,
    attempt: running ?? recentDone,
  })

  const check = () => {
    setChecking(true)
    void query.refetch().then((r) => {
      setChecking(false)
      toast.add(
        r.isError
          ? { title: t('status.checkFailedTitle'), type: 'error' }
          : { title: t('status.checked'), type: 'info' },
      )
    })
  }

  const showChanges = status.update_available && !edge
  return (
    <UpdateStatusView
      state={state}
      currentVersion={status.current_version}
      latestVersion={status.latest_version}
      edge={edge}
      lastChecked={
        query.dataUpdatedAt > 0
          ? formatRelative(new Date(query.dataUpdatedAt))
          : null
      }
      checking={checking}
      errorMessage={query.error?.message}
      releaseUrl={status.release_url}
      onCheck={check}
    >
      <UpgradeStepper steps={steps} />
      {showChanges ? <UpdateChanges status={status} /> : null}
      <UpgradeNow />
    </UpdateStatusView>
  )
}
