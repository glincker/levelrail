import { useAppNetwork } from '../../queries/appNetwork'
import { FallbackBetaNote } from '../FallbackBetaNote'

/** AutoUrlBetaNote marks the app's automatic sslip.io URL as a beta test address. */
export function AutoUrlBetaNote({ appName }: { appName: string }) {
  const { data: network } = useAppNetwork(appName)
  return <FallbackBetaNote url={network?.fallback_url} />
}
