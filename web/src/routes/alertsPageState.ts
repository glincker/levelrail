import { useAlertHistory } from '../queries/alertNoise'
import { useInbox } from '../queries/inbox'
import { useMaintenanceWindows, useSilences } from '../queries/alertNoise'

export type AlertsPageState = 'loading' | 'empty' | 'idle' | 'active'

// empty: nothing ever arrived or was configured; active wins over idle.
export function alertsPageState(input: {
  loading: boolean
  unread: number
  hasInbox: boolean
  hasHistory: boolean
  hasSilences: boolean
  hasWindows: boolean
}): AlertsPageState {
  if (input.loading) return 'loading'
  if (input.unread > 0) return 'active'
  const any =
    input.hasInbox || input.hasHistory || input.hasSilences || input.hasWindows
  return any ? 'idle' : 'empty'
}

// Reuses the panels' own query keys so nothing is fetched twice.
export function useAlertsPageState(): {
  state: AlertsPageState
  unread: number
} {
  const inbox = useInbox(false)
  const history = useAlertHistory({ app: undefined, outcome: '', event: '' })
  const silences = useSilences(false)
  const windows = useMaintenanceWindows()
  const items = inbox.data?.pages.flat() ?? []
  const unread = items.filter((i) => !i.read).length
  const state = alertsPageState({
    loading:
      inbox.isLoading ||
      history.isLoading ||
      silences.isLoading ||
      windows.isLoading,
    unread,
    hasInbox: items.length > 0,
    hasHistory: (history.data?.length ?? 0) > 0,
    hasSilences: (silences.data?.length ?? 0) > 0,
    hasWindows: (windows.data?.length ?? 0) > 0,
  })
  return { state, unread }
}
