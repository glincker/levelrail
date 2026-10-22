import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from '../../ui/toast'
import { useIsRoot } from '../../../hooks/useIsRoot'
import { selfUpgradeStatusQueryOptions } from '../../../queries/selfUpgrade'
import { isSeen, markSeen } from '../../../lib/upgradeSeen'

const OUTCOME_TOAST_MAX_AGE_MS = 24 * 60 * 60_000

// Tells the user once, on the first load after the control plane restarted
// into the new version, that the upgrade finished.
export function UpgradeOutcomeToast() {
  const { t } = useTranslation('updates')
  const isRoot = useIsRoot()
  const last = useQuery({
    ...selfUpgradeStatusQueryOptions(false),
    enabled: isRoot,
  }).data?.last_finished

  useEffect(() => {
    if (!last || last.outcome !== 'succeeded' || isSeen('toast', last.id))
      return
    markSeen('toast', last.id)
    const age = Date.now() - Date.parse(last.finished_at || last.started_at)
    if (age > OUTCOME_TOAST_MAX_AGE_MS) return
    toast.add({
      title: t('result.toast', { to: last.to_version }),
      type: 'success',
    })
  }, [last, t])
  return null
}
