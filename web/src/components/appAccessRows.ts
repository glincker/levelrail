import type { AppUrlAddress, AppUrls } from '../queries/appUrls'

/** URL "Open app" should use: domain first, then LAN, then localhost. */
export function pickOpenUrl(urls: AppUrls | undefined): string | null {
  if (!urls) return null
  const hit = (kind: AppUrlAddress['kind']) =>
    urls.addresses.find((a) => a.kind === kind && a.url)?.url ?? null
  if (!urls.running) return hit('domain')
  return hit('domain') ?? hit('lan') ?? hit('public') ?? hit('loopback')
}
