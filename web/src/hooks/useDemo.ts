import { useSyncExternalStore } from 'react'
import { getDemoInfo, subscribeDemo, type DemoInfo } from '../lib/demoStore'

export function useDemoInfo(): DemoInfo {
  return useSyncExternalStore(subscribeDemo, getDemoInfo)
}

export function useIsDemo(): boolean {
  return useDemoInfo().enabled
}
