import { useEffect, useRef, useState } from 'react'
import { detectDnsProvider, type DnsDetection } from '@/queries/domainDns'
import { DETECT_CONCURRENCY } from './providers'

type Result = DnsDetection | 'error'

/** Runs detection with a small worker pool so a long domain list never floods the API. */
export function useDetections(domains: string[], enabled: boolean) {
  const [results, setResults] = useState<Record<string, Result>>({})
  const startedRef = useRef(new Set<string>())
  useEffect(() => {
    if (!enabled) return
    const started = startedRef.current
    const queue = domains.filter((d) => !started.has(d))
    queue.forEach((d) => started.add(d))
    let cancelled = false
    const worker = async () => {
      for (let d = queue.shift(); d !== undefined; d = queue.shift()) {
        let r: Result
        try {
          r = await detectDnsProvider(d)
        } catch {
          r = 'error'
        }
        if (cancelled) return
        setResults((prev) => ({ ...prev, [d]: r }))
      }
    }
    void Promise.all(
      Array.from(
        { length: Math.min(DETECT_CONCURRENCY, queue.length) },
        worker,
      ),
    )
    return () => {
      cancelled = true
      // Unfinished names are retried by the next run.
      queue.forEach((d) => {
        if (!(d in results)) started.delete(d)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [domains.join('|'), enabled])
  return results
}
