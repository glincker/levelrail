export interface DemoInfo {
  enabled: boolean
  resetsAt?: string
  installCommand?: string
  repoUrl?: string
  docsUrl?: string
  version?: string
  username?: string
  password?: string
  humanCheck?: boolean
}

type Listener = () => void

const DISABLED: DemoInfo = { enabled: false }
let current: DemoInfo = DISABLED
const listeners = new Set<Listener>()

// Plain store so Button and ConfirmDialog can read demo state without a provider or QueryClient.
export function setDemoInfo(next: DemoInfo): void {
  current = next
  listeners.forEach((l) => {
    l()
  })
}

export function getDemoInfo(): DemoInfo {
  return current
}

export function subscribeDemo(listener: Listener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export const DEMO_ERROR_PREFIX = 'demo_'
