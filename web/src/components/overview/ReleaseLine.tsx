import { useTranslation } from 'react-i18next'
import { PackageGlassIcon } from '@thesvg/ui-react'
import { RelativeTime, InfoTip } from '@/components/kit'
import { shortDigest, unpinnedImage } from '../../lib/imageDigest'
import { imageTagOf } from './imageTag'
import { commitSubject } from '../../lib/deploymentPresentation'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttempt } from '../../types/deployAttempt'

export function ReleaseLine({
  app,
  latest,
}: {
  app: Pick<AppDetail, 'image' | 'image_digest'>
  latest?: DeployAttempt
}) {
  const { t } = useTranslation('deploys')
  const digest = app.image_digest ?? latest?.image_digest
  const tag = imageTagOf(app.image)
  const subject =
    (latest?.commit_message && commitSubject(latest.commit_message)) ||
    (/^[0-9a-f]{40}$/.test(tag) ? tag.slice(0, 7) : tag)
  return (
    <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
      <PackageGlassIcon className="size-4.5" aria-hidden="true" />
      <span
        className="max-w-full truncate font-medium text-foreground"
        title={latest?.commit_message || app.image}
      >
        {subject}
      </span>
      {latest?.commit_sha ? (
        <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {latest.commit_sha.slice(0, 7)}
        </span>
      ) : null}
      {digest ? (
        <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {shortDigest(digest)}
        </span>
      ) : null}
      {latest ? (
        <span>
          {t('releaseLine.deployed')}{' '}
          <RelativeTime at={latest.started_at} live />
          {latest.source && t(`releaseLine.source.${latest.source}`, '')
            ? ` ${t('releaseLine.from')} ${t(`releaseLine.source.${latest.source}`)}`
            : ''}
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
