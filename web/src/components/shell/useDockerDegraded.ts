import { useState } from 'react'
import { useDockerDegradedPoll } from '../../queries/dockerIntegration'

// Dismissal is keyed to the outage start, so a new outage shows the banner again.
export function useDockerDegraded() {
  const health = useDockerDegradedPoll()
  const [dismissedSince, setDismissedSince] = useState<string>()
  const since = health.data?.unresponsive
    ? (health.data.unresponsive_since ?? 'unknown')
    : undefined
  return {
    active: since !== undefined,
    visible: since !== undefined && dismissedSince !== since,
    checking: health.isFetching,
    recheck: () => void health.refetch(),
    dismiss: () => {
      setDismissedSince(since)
    },
  }
}
