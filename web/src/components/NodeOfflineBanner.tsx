import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { buttonVariants } from '@/components/ui/button'
import { useNow } from '../hooks/useNow'
import { formatDurationMs } from '../lib/deployDuration'
import { nodeListQueryOptions } from '../queries/nodes'
import { isNodeOfflineStatus } from '../lib/appStatus'
import type { ReconcileCondition } from '../types/deploy'

// Names the offline node on the app page, since the generic rollout state alone reads as a stuck deploy.
export function NodeOfflineBanner({
  conditions,
  nodeName,
}: {
  conditions: ReconcileCondition[]
  nodeName?: string
}) {
  const { t } = useTranslation('nodes')
  const offline = isNodeOfflineStatus(conditions)
  const now = useNow(30_000)
  const nodes = useQuery({ ...nodeListQueryOptions(), enabled: offline })
  if (!offline) return null
  const node = nodes.data?.find((n) => n.name === nodeName)
  const name = nodeName ?? node?.name ?? ''
  const seen = node?.last_seen_at ? Date.parse(node.last_seen_at) : NaN
  const since = Number.isNaN(seen) ? '' : (formatDurationMs(now - seen) ?? '')
  return (
    <Banner
      variant="critical"
      title={
        since
          ? t('offlineApp.title', { node: name, since })
          : t('offlineApp.titleUnknown', { node: name })
      }
      action={
        node ? (
          <Link
            to="/nodes/$id"
            params={{ id: node.id }}
            className={buttonVariants({ size: 'sm', variant: 'outline' })}
          >
            {t('offlineApp.view')}
          </Link>
        ) : null
      }
    >
      <p>{t('offlineApp.body')}</p>
    </Banner>
  )
}
