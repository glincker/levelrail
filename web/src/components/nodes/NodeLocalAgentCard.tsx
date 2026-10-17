import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { NodeResource } from '../../types/nodeDetail'
import { TrustItem } from './TrustItem'

// The control plane's own node runs inside the control plane process, so
// there is no network link to secure and no agent certificate: say that,
// and show nothing that would only read "Never" or "Unknown".
export function NodeLocalAgentCard({ node }: { node: NodeResource }) {
  const { t } = useTranslation('nodes')
  const { agent } = node
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('agent.localTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t('agent.localBody')}</p>
        <dl className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <TrustItem label={t('agent.version')}>
            {agent?.version || t('agent.notReported')}
          </TrustItem>
          <TrustItem label={t('agent.platform')}>
            {agent?.os
              ? `${agent.os}/${agent.arch ?? ''}`
              : t('agent.notReported')}
          </TrustItem>
        </dl>
      </CardContent>
    </Card>
  )
}
