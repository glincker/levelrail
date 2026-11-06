// Composes janitor policy and runtime settings reads and writes for the storage policy strip.

import {
  useJanitorStatus,
  useSaveJanitorPolicy,
  type JanitorPolicy,
} from './janitor'
import { useRuntimeSettings, useSetRuntimeSetting } from './runtimeSettings'

export function useStoragePolicy() {
  const janitor = useJanitorStatus()
  const settings = useRuntimeSettings()
  return {
    janitor: janitor.data?.policy ?? null,
    settings: settings.data?.settings,
    loading: janitor.isPending || settings.isPending,
  }
}

export function useSaveStorageSetting() {
  const mutation = useSetRuntimeSetting()
  // Ints travel as strings in the runtime settings API.
  return {
    isPending: mutation.isPending,
    save: (key: string, value: number) =>
      mutation.mutateAsync({ key, value: String(value) }),
  }
}

export function useSaveCleanupPolicy() {
  const mutation = useSaveJanitorPolicy()
  return {
    isPending: mutation.isPending,
    save: (current: JanitorPolicy, patch: Partial<JanitorPolicy>) =>
      mutation.mutateAsync({ ...current, ...patch }),
  }
}
