// The browser fires beforeinstallprompt once, early. This module is imported from main.tsx so the
// event is held until the dashboard shows its own install card.

interface InstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const DISMISS_KEY = 'lr:install-card-dismissed'
let deferred: InstallPromptEvent | null = null
const listeners = new Set<() => void>()

function notify() {
  listeners.forEach((fn) => fn())
}

if (typeof window !== 'undefined') {
  window.addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault()
    deferred = e as InstallPromptEvent
    notify()
  })
  window.addEventListener('appinstalled', () => {
    deferred = null
    notify()
  })
}

export function subscribeInstallPrompt(fn: () => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export function canPromptInstall(): boolean {
  return deferred !== null
}

export function isStandalone(): boolean {
  if (typeof window === 'undefined') return false
  const nav = window.navigator as Navigator & { standalone?: boolean }
  return (
    window.matchMedia('(display-mode: standalone)').matches ||
    nav.standalone === true
  )
}

// iOS Safari never fires beforeinstallprompt, so it gets written steps instead.
export function isIOSSafari(ua: string = navigator.userAgent): boolean {
  return /iphone|ipad|ipod/i.test(ua) && !/crios|fxios|edgios/i.test(ua)
}

export async function promptInstall(): Promise<boolean> {
  const ev = deferred
  if (!ev) return false
  await ev.prompt()
  const choice = await ev.userChoice
  deferred = null
  notify()
  return choice.outcome === 'accepted'
}

export function installCardDismissed(): boolean {
  try {
    return localStorage.getItem(DISMISS_KEY) === '1'
  } catch {
    return false
  }
}

export function dismissInstallCard() {
  try {
    localStorage.setItem(DISMISS_KEY, '1')
  } catch {
    // storage blocked: the card simply returns next visit
  }
  notify()
}
