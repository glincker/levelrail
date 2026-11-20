import { useTranslation } from 'react-i18next'
import { GitBranchGlassIcon, UploadGlassIcon } from '@thesvg/ui-react'
import { InfoTip, RelativeTime } from '@/components/kit'
import type { DeployAttempt } from '../../types/deployAttempt'

const GIT_SOURCES = new Set(['webhook', 'git_push', 'poll', 'pr_comment'])

/** One compact line for a static site's last deploy; hashes live in the tip. */
export function StaticDeployLine({ latest }: { latest?: DeployAttempt }) {
  const { t } = useTranslation('appOverview')
  if (!latest) return null
  const source = latest.source ?? 'other'
  const Icon = GIT_SOURCES.has(source) ? GitBranchGlassIcon : UploadGlassIcon
  const sha = latest.commit_sha ?? latest.image_digest
  return (
    <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      <span className="font-medium text-foreground">
        {t(`staticLine.source.${source}`, {
          defaultValue: t('staticLine.source.other'),
        })}
      </span>
      <span aria-hidden="true">&middot;</span>
      <RelativeTime at={latest.started_at} live />
      {sha ? (
        <InfoTip label={t('staticLine.details')}>
          <p className="font-mono break-all">{sha}</p>
          {latest.commit_message ? (
            <p className="mt-1 break-words">{latest.commit_message}</p>
          ) : null}
        </InfoTip>
      ) : null}
    </p>
  )
}
