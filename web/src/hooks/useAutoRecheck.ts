import { useEffect, useRef } from 'react'
import { recheckDelayMs } from '../lib/proxyIntegration'
import {
  useVerifyProxyDomain,
  type ProxyDomain,
} from '../queries/proxyIntegration'

// Rechecks with a growing delay while the server says a certificate is still
// expected, so the status settles without a manual click.
export function useAutoRecheck(row: ProxyDomain) {
  const verify = useVerifyProxyDomain()
  const attempt = useRef(0)
  const until = row.recheck_until ? Date.parse(row.recheck_until) : 0
  const checked = row.checked_at
  const { mutate, isPending } = verify
  const domain = row.domain
  useEffect(() => {
    if (!until || until <= Date.now() || isPending) return undefined
    const timer = setTimeout(() => {
      attempt.current += 1
      mutate(domain)
    }, recheckDelayMs(attempt.current))
    return () => clearTimeout(timer)
  }, [until, checked, isPending, mutate, domain])
}
