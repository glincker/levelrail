import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { RelativeTime, InfoTip } from '@/components/kit'
import { shortDigest, unpinnedImage } from '../../lib/imageDigest'
import { imageTagOf } from './imageTag'
import type { AppDetail } from '../../types/appDetail'
import type {
  DeployAttempt,
  DeployAttemptSource,
} from '../../types/deployAttempt'

const SOURCE_LABEL: Record<DeployAttemptSource, string> = {
  webhook: 'a git push',
  manual: 'the dashboard',
  image: 'an image deploy',
  auto_rollback: 'an automatic rollback',
}

export function ReleaseLine({
  app,
  latest,
}: {
  app: Pick<AppDetail, 'image' | 'image_digest'>
  latest?: DeployAttempt
}) {
  const digest = app.image_digest ?? latest?.image_digest
  return (
    <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
      <PackageIcon className="size-4" aria-hidden="true" />
      <span className="font-mono text-foreground">{imageTagOf(app.image)}</span>
      {digest ? (
        <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {shortDigest(digest)}
        </span>
      ) : null}
      {latest ? (
        <span>
          deployed <RelativeTime at={latest.started_at} live />
          {latest.source ? ` from ${SOURCE_LABEL[latest.source]}` : ''}
        </span>
      ) : null}
      <InfoTip label="Full image reference">
        <p className="break-all font-mono">{unpinnedImage(app.image)}</p>
        {digest ? (
          <p className="mt-1 break-all font-mono text-muted-foreground">
            {digest}
          </p>
        ) : null}
      </InfoTip>
    </p>
  )
}
