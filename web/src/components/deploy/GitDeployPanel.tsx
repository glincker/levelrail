import { useState } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import {
  GitBranchGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { RelativeTime } from '@/components/kit'
import { useProtectedEnvironment } from '../../queries/environments'
import { useGitSourceCommits } from '../../queries/gitSourceCommits'
import type { GitSourceCommitsResponse } from '../../queries/gitSourceCommits'
import type { AppDetail } from '../../types/appDetail'
import type { GitSourceResource } from '../../types/gitSource'
import { CommitRow, CommitStatusBadge } from './CommitRow'
import { DeployShortcuts } from './DeployShortcuts'
import { DeploySuggestions } from './DeploySuggestions'
import { PendingChangesCard } from './PendingChangesCard'
import { ProtectedDeployNotice } from './ProtectedDeployNotice'
import { useDeployActions } from './useDeployActions'

function liveStatusLine(
  t: TFunction<'smartDeploy'>,
  data: GitSourceCommitsResponse,
): string {
  if (data.up_to_date && data.head) {
    return t('git.statusUpToDate', { sha: data.head.sha.slice(0, 7) })
  }
  if (data.ahead_by !== undefined) {
    return t('git.statusAhead', { count: data.ahead_by })
  }
  if (data.deployed) return t('git.statusAheadUnknown')
  return t('git.statusNeverDeployed')
}

function BuildLine({ data }: { data: GitSourceCommitsResponse }) {
  const { t } = useTranslation('smartDeploy')
  const type = t(
    `git.buildTypes.${data.build_type}` as 'git.buildTypes.dockerfile',
    { defaultValue: data.build_type },
  )
  return (
    <p className="text-xs text-muted-foreground">
      {t('git.buildWith')} {type}
      {data.build.dockerfile_path
        ? `, ${t('git.dockerfile', { path: data.build.dockerfile_path })}`
        : ''}
      {data.build.context_dir && data.build.context_dir !== '.'
        ? `, ${t('git.contextDir', { dir: data.build.context_dir })}`
        : ''}
    </p>
  )
}

export function GitDeployPanel({
  app,
  source,
  onClose,
}: {
  app: AppDetail
  source: GitSourceResource
  onClose: () => void
}) {
  const { t } = useTranslation('smartDeploy')
  const [branch, setBranch] = useState('')
  const [ack, setAck] = useState(false)
  const query = useGitSourceCommits(app.name, branch, true)
  const protectedEnv = useProtectedEnvironment(app)
  const actions = useDeployActions({
    appName: app.name,
    source,
    onDone: onClose,
  })
  const activeBranch = branch || source.branch
  const needsAck = protectedEnv?.protected === true && !ack
  const blocked = actions.pending || needsAck
  const data = query.data

  const branches = data
    ? Array.from(new Set([source.branch, ...data.branches]))
    : [source.branch]

  return (
    <div className="space-y-4">
      <PendingChangesCard appName={app.name} />
      <div className="flex items-end gap-2">
        <div className="min-w-0 flex-1 space-y-1">
          <span className="block text-xs font-medium text-muted-foreground">
            {t('git.branch')}
          </span>
          <Select
            value={activeBranch}
            onValueChange={(next: string | null) => {
              setBranch(next && next !== source.branch ? next : '')
            }}
          >
            <SelectTrigger
              className="w-full font-mono"
              aria-label={t('git.branch')}
            >
              <GitBranchGlassIcon size="1.2em" aria-hidden="true" />
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {branches.map((b) => (
                <SelectItem key={b} value={b} className="font-mono">
                  {b}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('git.refresh')}
          disabled={query.isFetching}
          onClick={() => {
            void query.refetch()
          }}
        >
          <RotateCwGlassIcon size="1.2em" aria-hidden="true" />
        </Button>
      </div>
      {activeBranch !== source.branch ? (
        <p className="text-xs text-muted-foreground">
          {t('git.branchOther', { branch: source.branch })}
        </p>
      ) : null}

      {query.isError ? (
        <Alert variant="destructive">
          <AlertDescription className="space-y-2">
            <p>
              {t('git.loadFailed')}: {query.error.message}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  void query.refetch()
                }}
              >
                {t('git.retry')}
              </Button>
              <Button
                size="sm"
                disabled={blocked}
                onClick={() => {
                  actions.deployCommit(activeBranch)
                }}
              >
                {t('git.deployBranch', { branch: activeBranch })}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : null}

      {data ? (
        <>
          <section className="space-y-3 rounded-xl border border-border bg-card p-4">
            <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
              {t('git.headTitle', { branch: data.branch })}
            </h3>
            {data.head ? (
              <>
                <div className="min-w-0">
                  <p className="truncate text-sm">
                    {data.head.message || t('git.noMessage')}
                  </p>
                  <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                    <code className="font-mono">
                      {data.head.sha.slice(0, 7)}
                    </code>
                    {data.head.author ? <span>{data.head.author}</span> : null}
                    {data.head.authored_at ? (
                      <RelativeTime at={data.head.authored_at} />
                    ) : null}
                    <CommitStatusBadge status={data.head.deploy_status} />
                  </p>
                </div>
                <p className="text-xs">{liveStatusLine(t, data)}</p>
                <BuildLine data={data} />
                <Button
                  className="w-full"
                  disabled={blocked}
                  onClick={() => {
                    if (data.head) actions.deployCommit(data.head.sha)
                  }}
                >
                  <RocketGlassIcon size="1.2em" aria-hidden="true" />
                  {actions.buildPending
                    ? t('git.starting')
                    : t('git.deployHead')}
                </Button>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">
                {t('git.noCommits')}
              </p>
            )}
            {data.degraded ? (
              <p className="text-xs text-muted-foreground">
                {t('git.degraded', { branch: data.branch })}
              </p>
            ) : null}
          </section>

          <DeploySuggestions
            appName={app.name}
            suggestions={data.suggestions}
            onNavigate={onClose}
          />

          {protectedEnv?.protected ? (
            <ProtectedDeployNotice
              environmentName={protectedEnv.name}
              acknowledged={ack}
              onAcknowledgedChange={setAck}
            />
          ) : null}

          <DeployShortcuts
            last={data.last_successful}
            previous={data.previous_successful}
            disabled={blocked}
            pending={actions.imagePending}
            onDeployImage={(image) => {
              actions.deployImage(image, { confirm: ack })
            }}
          />

          {data.commits.length > 1 ? (
            <section className="space-y-1">
              <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
                {t('git.recent')}
              </h3>
              <ul className="divide-y divide-border">
                {data.commits.slice(1).map((c) => (
                  <CommitRow
                    key={c.sha}
                    commit={c}
                    disabled={blocked}
                    onDeploy={actions.deployCommit}
                  />
                ))}
              </ul>
            </section>
          ) : null}
        </>
      ) : null}
      {query.isLoading ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t('git.loading')}
        </p>
      ) : null}
    </div>
  )
}
