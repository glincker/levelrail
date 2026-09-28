import { RelativeTime } from '@/components/kit'
import type { Deployment } from '../../types/deployment'
import { queueLabel, shortId } from '../../lib/deploymentPresentation'

function DeployRefLink({
  id,
  onOpen,
}: {
  id: string
  onOpen: (id: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => {
        onOpen(id)
      }}
      className="rounded-sm font-mono underline underline-offset-4 outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
    >
      {shortId(id)}
    </button>
  )
}

/** Explains a queued, canceled or superseded deploy, with links to the deploys involved. */
export function DeploymentNote({
  d,
  onOpen,
}: {
  d: Deployment
  onOpen: (id: string) => void
}) {
  if (d.status === 'queued') {
    return (
      <span className="text-xs text-muted-foreground">
        {queueLabel(d)}
        {d.blocked_by && (
          <>
            {' '}
            (blocked by <DeployRefLink id={d.blocked_by} onOpen={onOpen} />)
          </>
        )}
      </span>
    )
  }
  if (d.status === 'canceled') {
    return (
      <span className="text-sm text-muted-foreground">
        {d.canceled_by ? `Canceled by ${d.canceled_by}` : 'Canceled'}
        {d.finished_at && (
          <>
            {', '}
            <RelativeTime at={d.finished_at} />
          </>
        )}
        {d.superseded_by && (
          <>
            {'. Replaced by '}
            <DeployRefLink id={d.superseded_by} onOpen={onOpen} />
          </>
        )}
        .
      </span>
    )
  }
  if (d.superseded_by) {
    return (
      <span className="text-sm text-muted-foreground">
        Superseded by <DeployRefLink id={d.superseded_by} onOpen={onOpen} />. A
        newer deploy for the same branch replaced this one before it built.
      </span>
    )
  }
  return null
}
