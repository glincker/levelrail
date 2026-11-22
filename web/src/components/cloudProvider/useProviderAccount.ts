import { useCallback, useSyncExternalStore } from 'react'
import type { NodeProviderAccount } from '../../types/nodeProvision'

const KEY = (provider: string) => `lr.cloudProvider.account.${provider}`
const listeners = new Set<() => void>()

function read(provider: string): string | null {
  try {
    return localStorage.getItem(KEY(provider))
  } catch {
    return null
  }
}

function subscribe(fn: () => void) {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

// The first active account is the default, so a stale or missing pick never selects nothing.
export function pickAccount(
  accounts: readonly NodeProviderAccount[],
  saved: string | null,
): NodeProviderAccount | null {
  return (
    accounts.find((a) => a.id === saved) ??
    accounts.find((a) => a.status === 'active' && !a.needs_relogin) ??
    accounts[0] ??
    null
  )
}

// Shared by the three sections through one store, so switching in one updates the others.
export function useProviderAccount(
  provider: string,
  accounts: readonly NodeProviderAccount[],
) {
  const saved = useSyncExternalStore(
    subscribe,
    () => read(provider),
    () => null,
  )
  const select = useCallback(
    (id: string) => {
      try {
        localStorage.setItem(KEY(provider), id)
      } catch {
        // storage can be blocked; the pick then lasts only until reload
      }
      listeners.forEach((fn) => {
        fn()
      })
    },
    [provider],
  )
  return { account: pickAccount(accounts, saved), select }
}
