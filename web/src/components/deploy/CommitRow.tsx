import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { RelativeTime } from '@/components/kit'
import { TONE_BADGE_VARIANT, type Tone } from '@/components/kit/tone'
import type {
  CommitDeployStatus,
  GitCommit,
} from '../../queries/gitSourceCommits'

const STATUS_TONE: Record<CommitDeployStatus, Tone> = {
  deployed: 'success',
  succeeded: 'neutral',
  failed: 'danger',
  running: 'info',
  queued: 'info',
  held: 'warning',
  canceled: 'neutral',
  superseded: 'neutral',
}

export function CommitStatusBadge({ status }: { status?: CommitDeployStatus }) {
  const { t } = useTranslation('smartDeploy')
  if (!status) return null
  return (
    <Badge variant={TONE_BADGE_VARIANT[STATUS_TONE[status]]}>
      {t(`git.status.${status}` as const)}
    </Badge>
  )
}

export function CommitRow({
  commit,
  disabled,
  onDeploy,
}: {
  commit: GitCommit
  disabled: boolean
  onDeploy: (sha: string) => void
}) {
  const { t } = useTranslation('smartDeploy')
  const isLive = commit.deploy_status === 'deployed'
  return (
    <li className="flex items-center gap-3 py-2">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {commit.message || t('git.noMessage')}
        </p>
        <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
          <code className="font-mono">{commit.sha.slice(0, 7)}</code>
          {commit.author ? <span>{commit.author}</span> : null}
          {commit.authored_at ? <RelativeTime at={commit.authored_at} /> : null}
        </p>
      </div>
      <CommitStatusBadge status={commit.deploy_status} />
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => {
          onDeploy(commit.sha)
        }}
        aria-label={t('git.deployCommit', { sha: commit.sha.slice(0, 7) })}
      >
        {isLive ? t('git.redeploy') : t('git.deploy')}
      </Button>
    </li>
  )
}
