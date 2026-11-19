import { installDemoFetchGuard, type DemoBlock } from './demoFetchGuard'
import {
  createDemoToastGate,
  installUserActionTracking,
  type DemoToastGate,
} from './demoToast'

export type DemoBlockHandler = (b: DemoBlock, gate: DemoToastGate) => void

let gate: DemoToastGate | null = null
let handler: DemoBlockHandler | null = null
const pending: DemoBlock[] = []

/** Installs the write guard and intent tracking eagerly; blocks seen before the lazy chrome loads are queued. */
export function startDemoGuard(): () => void {
  gate = createDemoToastGate()
  const g = gate
  const stop = installUserActionTracking(g)
  installDemoFetchGuard((b) => {
    if (handler) handler(b, g)
    else pending.push(b)
  })
  return stop
}

/** Registers the chrome's handler and replays blocks queued before it mounted. */
export function setDemoBlockHandler(h: DemoBlockHandler | null): void {
  handler = h
  if (!h || !gate) return
  for (const b of pending.splice(0)) h(b, gate)
}
