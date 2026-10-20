import { useAppNetwork } from '../../queries/appNetwork'
import type { AppDetail } from '../../types/appDetail'

/** Public URL of the app: primary domain, then the fallback URL, then the host port. */
export function appUrlFrom(
  app: Pick<AppDetail, 'domains'>,
  network?: { running: boolean; host_port?: number; fallback_url?: string },
  hostname = window.location.hostname,
): string | null {
  const domain = app.domains?.[0]
  if (domain) return `https://${domain}`
  if (network?.fallback_url) return network.fallback_url
  if (network?.running && network.host_port) {
    return `http://${hostname}:${network.host_port}`
  }
  return null
}

export function useAppUrl(app: AppDetail): string | null {
  const { data: network } = useAppNetwork(app.name)
  return appUrlFrom(app, network)
}
