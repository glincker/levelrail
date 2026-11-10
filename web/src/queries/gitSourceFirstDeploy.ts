import { useEffect, useState } from 'react'
import type { Deployment } from '../types/deployment'
import { parseDeploymentEvent } from '../hooks/useDeploymentsStream'

const STREAM_URL = '/api/v1/deployments/stream'

// Follows the first deploy after Deploy now on the shared deployments stream,
// one EventSource and no polling; the newest event for the app wins.
export function useFirstDeployResult(
  appName: string,
  since: string | null,
): Deployment | null {
  const [latest, setLatest] = useState<Deployment | null>(null)
  useEffect(() => {
    if (!since || typeof EventSource === 'undefined') return
    const source = new EventSource(STREAM_URL)
    source.onmessage = (msg: MessageEvent<string>) => {
      const ev = parseDeploymentEvent(msg.data)
      if (!ev || ev.deployment.app !== appName) return
      if (ev.deployment.started_at && ev.deployment.started_at < since) return
      setLatest(ev.deployment)
      if (ev.type === 'finished') source.close()
    }
    return () => source.close()
  }, [appName, since])
  return latest
}

export function isFinalDeployStatus(status: Deployment['status']): boolean {
  return status !== 'building' && status !== 'queued'
}
