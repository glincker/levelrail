import { useQuery } from '@tanstack/react-query'
import { liveBadgesQueryOptions } from '../../queries/goLiveChecklist'
import { useAppNetwork } from '../../queries/appNetwork'
import { useStuck } from '../../queries/stuck'
import {
  shareableUrl,
  urlReachability,
  type UrlReachability,
} from '../../lib/urlReachability'
import type { AppDetail } from '../../types/appDetail'
import { pickAppLiveEntry } from '../golive/liveState'

export interface AppUrlReach {
  reach: UrlReachability
  /** Address a phone can open: the URL itself, or a public fallback for a local one. */
  shareUrl: string | null
}

/** Combines the page's own status fields (network, stuck report, DNS state) into one honest answer. */
export function useUrlReachability(
  app: Pick<AppDetail, 'name' | 'suspended'>,
  url: string | null,
): AppUrlReach {
  const { data: network } = useAppNetwork(app.name)
  const stuck = useStuck('app', app.name).data
  const { data: badges } = useQuery(liveBadgesQueryOptions())
  const live = pickAppLiveEntry(badges, app.name)
  const reach = urlReachability(app, network, {
    url,
    ready: stuck ? stuck.ready : true,
    liveState: live?.state,
  })
  return { reach, shareUrl: shareableUrl(url, reach, network) }
}
