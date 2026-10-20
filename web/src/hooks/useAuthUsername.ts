import { useSyncExternalStore } from 'react'
import { getStoredUsername, subscribeStoredUsername } from '../lib/authStore'

// React-facing wrapper around lib/authStore's plain get/subscribe pair.
// useSyncExternalStore (not useState+useEffect) so a login in one
// component (LoginForm) and a read in another (the app shell's nav in
// routes/__root.tsx) stay in sync without a shared context provider: the
// store itself is the single source of truth, this hook just subscribes
// to it.
export function useAuthUsername(): string | null {
  return useSyncExternalStore(subscribeStoredUsername, getStoredUsername)
}
