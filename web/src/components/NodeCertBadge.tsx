import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import type {
  NodeAgentResource,
  NodeCertResource,
  NodeCertState,
} from '../types/nodeCert'
import { nodeCertLabel } from '../lib/nodeCertLabel'

const CERT_VARIANT: Record<
  NodeCertState,
  VariantProps<typeof badgeVariants>['variant']
> = {
  ok: 'outline',
  expiring: 'warning',
  critical: 'destructive',
  expired: 'destructive',
  revoked: 'destructive',
  unknown: 'muted',
}

// showOk=false hides a healthy certificate where only problems matter.
export function NodeCertBadge({
  cert,
  showOk = true,
}: {
  cert?: NodeCertResource
  showOk?: boolean
}) {
  if (!cert || (cert.state === 'ok' && !showOk)) {
    return null
  }
  return (
    <Badge
      variant={CERT_VARIANT[cert.state]}
      title={cert.not_after ? new Date(cert.not_after).toLocaleString() : ''}
    >
      {nodeCertLabel(cert)}
    </Badge>
  )
}

export function AgentOutdatedBadge({ agent }: { agent?: NodeAgentResource }) {
  if (!agent?.outdated) {
    return null
  }
  return (
    <Badge
      variant="warning"
      title={`Minimum supported agent version is ${agent.min_version ?? ''}`}
    >
      Agent {agent.version ?? 'version unknown'} outdated
    </Badge>
  )
}
