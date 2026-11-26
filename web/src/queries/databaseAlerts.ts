import { useMemo } from 'react'
import { deriveDatabaseAlerts } from '../lib/databaseHealthAlerts'
import { useDefaultAlerts } from './alertDefaults'
import { useAlertHistory } from './alertNoise'

// Reuses the default-alert state and alert history; there is no per-database
// alert endpoint, so rows are matched to the database client side.
export function useDatabaseAlerts(database: string) {
  const defaults = useDefaultAlerts()
  const history = useAlertHistory({ event: 'fired', limit: 200 })
  const rows = useMemo(
    () =>
      deriveDatabaseAlerts(defaults.data ?? [], history.data ?? [], database),
    [defaults.data, history.data, database],
  )
  return {
    rows,
    firing: rows.filter((r) => r.state === 'firing'),
    isLoading: defaults.isLoading,
    isError: defaults.isError,
  }
}
