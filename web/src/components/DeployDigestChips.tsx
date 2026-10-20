import {
  FingerprintIcon,
  CheckCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { DeployAttempt } from '../types/deployAttempt'
import { shortDigest } from '../lib/imageDigest'

// Digest reasons that mean the registry was not consulted for this deploy.
const DEGRADED_REASONS = new Set(['PullFailedUsingCached', 'Unresolved'])

const REASON_HINT: Record<string, string> = {
  Resolved: 'Resolved from the registry at deploy time',
  AlreadyPinned: 'The deployed reference already carried this digest',
  LocalBuild: 'Image ID of the image this deploy built',
  PullFailedUsingCached:
    'The registry was unreachable, so the locally cached image was deployed',
  Unresolved:
    'Neither the registry nor the local cache knew this tag; the node resolves it when it starts the container',
}

export function DigestChip({
  digest,
  reason,
}: {
  digest?: string
  reason?: string
}) {
  if (!digest && !reason) return null
  const degraded = reason ? DEGRADED_REASONS.has(reason) : false
  const hint = [digest, reason ? (REASON_HINT[reason] ?? reason) : undefined]
    .filter(Boolean)
    .join('\n')
  return (
    <Badge
      variant={degraded ? 'warning' : 'outline'}
      className="shrink-0 font-mono"
      title={hint}
    >
      {degraded ? (
        <WarningIcon className="size-3" />
      ) : (
        <FingerprintIcon className="size-3" />
      )}
      {digest ? shortDigest(digest) : 'no digest'}
      {degraded ? (
        <span className="font-sans">
          {reason === 'Unresolved' ? 'unresolved' : 'cached'}
        </span>
      ) : null}
    </Badge>
  )
}

// RolloutChip shows what the application controller last saw running.
export function RolloutChip({ attempt }: { attempt: DeployAttempt }) {
  if (attempt.rollout_state === 'serving') {
    return (
      <Badge
        variant="success"
        className="shrink-0"
        title={attempt.running_image_id}
      >
        <CheckCircleIcon className="size-3" />
        Serving
      </Badge>
    )
  }
  if (attempt.rollout_state === 'mismatch') {
    return (
      <Badge
        variant="destructive"
        className="shrink-0"
        title={`Running ${attempt.running_image_id ?? 'unknown'} instead of the deployed image`}
      >
        <WarningIcon className="size-3" />
        Image mismatch
      </Badge>
    )
  }
  return null
}
