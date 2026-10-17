import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useIsRoot } from '../hooks/useIsRoot'
import { useNodeTrust } from '../queries/nodeTrust'
import type { NodeResource } from '../types/nodeDetail'
import { AgentOutdatedBadge, NodeCertBadge } from './NodeCertBadge'
import { NodeLocalAgentCard } from './nodes/NodeLocalAgentCard'
import { NodeSecurityTimeline } from './nodes/NodeSecurityTimeline'
import { NodeTrustDetails } from './nodes/NodeTrustDetails'
import { RenewNowDialog } from './nodes/RenewNowDialog'
import { TrustStatusLine } from './nodes/TrustStatusLine'
import { ReenrollNodeDialog } from './ReenrollNodeDialog'
import { RevokeNodeCertDialog } from './RevokeNodeCertDialog'

// Agent identity on the node detail page. The control plane's own node has
// no agent link to secure and gets a plain explanation; every other node
// shows what the control plane actually verified, from real events.
export function NodeAgentCard({
  node,
  openReenroll = false,
}: {
  node: NodeResource
  openReenroll?: boolean
}) {
  const { t } = useTranslation('nodes')
  const isRoot = useIsRoot()
  const trust = useNodeTrust(node.id, isRoot && !node.is_local)
  if (node.is_local) {
    return <NodeLocalAgentCard node={node} />
  }
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex flex-wrap items-center gap-2">
          {t('agent.title')}
          <NodeCertBadge cert={node.cert} />
          <AgentOutdatedBadge agent={node.agent} />
        </CardTitle>
        {isRoot ? (
          <div className="flex flex-wrap items-center gap-2">
            <RenewNowDialog node={node} />
            <ReenrollNodeDialog node={node} defaultOpen={openReenroll} />
            <RevokeNodeCertDialog node={node} />
          </div>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-5">
        {!isRoot ? (
          <p className="text-sm text-muted-foreground">{t('trust.rootOnly')}</p>
        ) : trust.isError ? (
          <p className="text-sm text-tone-danger">{t('trust.loadError')}</p>
        ) : trust.data ? (
          <>
            <TrustStatusLine trust={trust.data} />
            <NodeTrustDetails node={node} trust={trust.data} />
            <NodeSecurityTimeline nodeId={node.id} />
          </>
        ) : (
          <p className="text-sm text-muted-foreground">{t('trust.loading')}</p>
        )}
      </CardContent>
    </Card>
  )
}
