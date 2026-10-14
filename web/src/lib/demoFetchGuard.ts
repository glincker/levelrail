import { DEMO_ERROR_PREFIX } from './demoStore'

export interface DemoBlock {
  code: string
  message: string
}

let installed = false

// Wraps fetch once so any blocked write, from any fetcher, surfaces through one handler.
export function installDemoFetchGuard(onBlocked: (b: DemoBlock) => void): void {
  if (installed || typeof window === 'undefined') return
  installed = true
  const original = window.fetch.bind(window)
  window.fetch = async (...args: Parameters<typeof fetch>) => {
    const res = await original(...args)
    if (res.status === 403) {
      void res
        .clone()
        .json()
        .then((body: { error?: string; message?: string } | null) => {
          if (body?.error?.startsWith(DEMO_ERROR_PREFIX)) {
            onBlocked({ code: body.error, message: body.message ?? '' })
          }
        })
        .catch(() => undefined)
    }
    return res
  }
}
