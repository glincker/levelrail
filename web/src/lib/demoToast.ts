export const DEMO_TOAST_ID = 'demo-readonly'

const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS'])
const ACTION_TARGET =
  'button, [role="button"], [role="menuitem"], input[type="submit"], form'

export interface DemoToastGateOptions {
  intervalMs?: number
  actionWindowMs?: number
  now?: () => number
}

export interface DemoToastGate {
  noteUserAction: () => void
  accept: (method: string) => boolean
}

function envMs(raw: unknown, fallback: number): number {
  if (typeof raw !== 'string' || raw === '') return fallback
  const n = Number(raw)
  return Number.isFinite(n) && n >= 0 ? n : fallback
}

/** Decides whether a blocked request deserves a notice: user-initiated writes only, at most one per interval. */
export function createDemoToastGate(
  opts: DemoToastGateOptions = {},
): DemoToastGate {
  const intervalMs =
    opts.intervalMs ?? envMs(import.meta.env.VITE_DEMO_TOAST_INTERVAL_MS, 10000)
  const windowMs =
    opts.actionWindowMs ??
    envMs(import.meta.env.VITE_DEMO_ACTION_WINDOW_MS, 2000)
  const now = opts.now ?? Date.now
  let lastAction = -Infinity
  let lastShown = -Infinity
  return {
    noteUserAction() {
      lastAction = now()
    },
    accept(method) {
      if (SAFE_METHODS.has(method.toUpperCase())) return false
      const t = now()
      if (t - lastAction > windowMs) return false
      if (t - lastShown < intervalMs) return false
      lastShown = t
      return true
    },
  }
}

interface ToastLike<T> {
  add: (o: T & { id: string }) => unknown
  close: (id: string) => void
}

/** showDemoToast closes any visible copy first, so a repeat replaces the toast instead of stacking. */
export function showDemoToast<T>(manager: ToastLike<T>, options: T): void {
  manager.close(DEMO_TOAST_ID)
  manager.add({ ...options, id: DEMO_TOAST_ID })
}

function isUserActionKey(e: KeyboardEvent): boolean {
  return e.key === 'Enter' || e.key === ' '
}

/** installUserActionTracking marks clicks on buttons and form submits as user intent; polling and mount-time writes never match. */
export function installUserActionTracking(
  gate: DemoToastGate,
  doc: Document = document,
): () => void {
  const onPointer = (e: Event) => {
    if (e.target instanceof Element && e.target.closest(ACTION_TARGET)) {
      gate.noteUserAction()
    }
  }
  const onKey = (e: KeyboardEvent) => {
    if (isUserActionKey(e)) onPointer(e)
  }
  const onSubmit = () => {
    gate.noteUserAction()
  }
  doc.addEventListener('click', onPointer, true)
  doc.addEventListener('keydown', onKey, true)
  doc.addEventListener('submit', onSubmit, true)
  return () => {
    doc.removeEventListener('click', onPointer, true)
    doc.removeEventListener('keydown', onKey, true)
    doc.removeEventListener('submit', onSubmit, true)
  }
}
