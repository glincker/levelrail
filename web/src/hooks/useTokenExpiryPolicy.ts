import { useRuntimeSettings } from '../queries/runtimeSettings'
import { DEFAULT_SOON_DAYS, soonDaysFromSetting } from '../lib/tokenExpiry'

const WINDOWS_KEY = 'credential_expiry.reminder_windows_days'
const GRACE_KEY = 'credential_expiry.rotation_grace'

export interface TokenExpiryPolicy {
  // Largest reminder window, the point where a token reads "expires soon".
  soonDays: number
  // Display forms, null until the settings load or when the caller cannot read them.
  windows: string | null
  grace: string | null
}

// Read only: the values are edited under runtime settings.
export function useTokenExpiryPolicy(): TokenExpiryPolicy {
  const settings = useRuntimeSettings().data?.settings
  const windows = settings?.find((s) => s.key === WINDOWS_KEY)?.value
  const grace = settings?.find((s) => s.key === GRACE_KEY)?.value
  return {
    soonDays:
      windows === undefined ? DEFAULT_SOON_DAYS : soonDaysFromSetting(windows),
    windows: windows === undefined ? null : windowsLabel(windows),
    grace: typeof grace === 'string' ? grace : null,
  }
}

function windowsLabel(value: unknown): string {
  return Array.isArray(value) ? value.join(', ') : String(value)
}
