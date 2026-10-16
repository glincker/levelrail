import { useAppMaintenance } from '../queries/appMaintenance'

// True while any domain of the app is in maintenance.
export function useAppInMaintenance(appName: string): boolean {
  const { data } = useAppMaintenance(appName)
  return data?.enabled ?? false
}
