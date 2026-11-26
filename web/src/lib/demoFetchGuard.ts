import { DEMO_ERROR_PREFIX, getDemoInfo } from './demoStore'
import { noteDemoRefusal } from './demoToast'

export interface DemoBlock {
  code: string
  message: string
  method: string
  path: string
}

let installed = false

// Wraps fetch once so any blocked write, from any fetcher, surfaces through one handler.
export function installDemoFetchGuard(onBlocked: (b: DemoBlock) => void): void {
  if (installed || typeof window === 'undefined') return
  installed = true
  const original = window.fetch.bind(window)
  window.fetch = async (...args: Parameters<typeof fetch>) => {
    const res = await original(...args)
    const req = args[0]
    const method = (
      args[1]?.method ?? (req instanceof Request ? req.method : 'GET')
    ).toUpperCase()
    const rawUrl = req instanceof Request ? req.url : String(req)
    if (res.status === 403) {
      // Sync mark: the body check below is async, so a custom onError toast would win the race.
      if (getDemoInfo().enabled && method !== 'GET') noteDemoRefusal()
      void res
        .clone()
        .json()
        .then((body: { error?: string; message?: string } | null) => {
          if (body?.error?.startsWith(DEMO_ERROR_PREFIX)) {
            onBlocked({
              code: body.error,
              message: body.message ?? '',
              method,
              path: new URL(rawUrl, window.location.origin).pathname,
            })
          }
        })
        .catch(() => undefined)
    }
    return res
  }
}
