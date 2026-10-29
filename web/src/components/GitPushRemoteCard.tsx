import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CheckGlassIcon,
  CopyGlassIcon,
  GitBranchGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useExperimentalFeatures } from '../hooks/useExperimental'
import { BETA_FEATURES_PATH, lockedSearch } from '../lib/experimental'
import {
  useGitRemote,
  useIssueGitRemote,
  useRevokeGitRemote,
  useSetGitRemoteBranch,
} from '../queries/gitRemote'
import type { GitRemoteResource } from '../types/gitRemote'

function CopyLine({ text, label }: { text: string; label: string }) {
  const { t } = useTranslation('gitBuild')
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
      <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
        {text}
      </code>
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-label={label}
        onClick={() => {
          void navigator.clipboard.writeText(text).then(() => {
            setCopied(true)
          })
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.2em" />
        ) : (
          <CopyGlassIcon size="1.2em" />
        )}
        {copied ? t('push.copied') : t('push.copy')}
      </Button>
    </div>
  )
}

function BranchForm({
  appName,
  remote,
}: {
  appName: string
  remote: GitRemoteResource
}) {
  const { t } = useTranslation('gitBuild')
  const save = useSetGitRemoteBranch(appName)
  const [branch, setBranch] = useState(remote.branch)
  const dirty = branch.trim() !== remote.branch
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(branch.trim(), {
          onSuccess: () => {
            toast.add({ title: t('push.branchSaved'), type: 'success' })
          },
        })
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="git-push-branch">{t('push.branchLabel')}</Label>
        <Input
          id="git-push-branch"
          value={branch}
          onChange={(e) => {
            setBranch(e.target.value)
          }}
          className="w-56"
        />
      </div>
      <Button type="submit" size="sm" disabled={!dirty || save.isPending}>
        {save.isPending ? t('saving') : t('push.saveBranch')}
      </Button>
      {save.isError ? (
        <p className="w-full text-sm text-destructive">{save.error.message}</p>
      ) : null}
    </form>
  )
}

export function GitPushRemoteCard({ appName }: { appName: string }) {
  const { t } = useTranslation('gitBuild')
  const enabled = useExperimentalFeatures().includes('git-push')
  const remote = useGitRemote(appName, enabled)
  const issue = useIssueGitRemote(appName)
  const revoke = useRevokeGitRemote(appName)
  const [issued, setIssued] = useState<GitRemoteResource | null>(null)
  const [confirm, setConfirm] = useState<'rotate' | 'revoke' | null>(null)

  const header = (
    <CardHeader>
      <CardTitle className="flex items-center gap-2">
        <GitBranchGlassIcon className="size-4.5 text-muted-foreground" />
        {t('push.title')}
      </CardTitle>
      <CardDescription>{t('push.description')}</CardDescription>
    </CardHeader>
  )

  if (!enabled) {
    return (
      <Card>
        {header}
        <CardContent className="space-y-3">
          <p className="text-sm text-muted-foreground">{t('push.off')}</p>
          <Button
            variant="outline"
            size="sm"
            render={
              <Link to={BETA_FEATURES_PATH} search={lockedSearch('git-push')} />
            }
          >
            {t('push.turnOn')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  const data = remote.data
  const runIssue = () => {
    issue.mutate(undefined, {
      onSuccess: (res) => {
        setIssued(res)
        setConfirm(null)
        toast.add({ title: t('push.tokenIssued'), type: 'success' })
      },
    })
  }

  return (
    <Card>
      {header}
      <CardContent className="space-y-4">
        {remote.isError ? (
          <p className="text-sm text-destructive">{remote.error.message}</p>
        ) : null}
        {issued?.add_command ? (
          <div className="space-y-2" role="status">
            <p className="text-sm font-medium">{t('push.runThis')}</p>
            <CopyLine text={issued.add_command} label={t('push.copyCommand')} />
            <p className="text-xs text-muted-foreground">
              {t('push.shownOnce', {
                remote: issued.remote_name,
                branch: issued.branch,
              })}
            </p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setIssued(null)
              }}
            >
              {t('push.done')}
            </Button>
          </div>
        ) : null}

        {data && !data.configured && !issued ? (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {t('push.notEnabled')}
            </p>
            <Button size="sm" disabled={issue.isPending} onClick={runIssue}>
              {issue.isPending ? t('push.enabling') : t('push.enable')}
            </Button>
          </div>
        ) : null}

        {data?.configured ? (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label>{t('push.remoteUrl')}</Label>
              <CopyLine
                text={`git remote add ${data.remote_name} ${data.url}`}
                label={t('push.copyRemote')}
              />
              <p className="text-xs text-muted-foreground">
                {t('push.credentials', { user: data.username })}
              </p>
            </div>
            <BranchForm key={data.branch} appName={appName} remote={data} />
            <p className="text-sm text-muted-foreground">
              {data.last_push_at
                ? t('push.lastPush', {
                    when: new Date(data.last_push_at).toLocaleString(),
                    commit: (data.last_push_commit ?? '').slice(0, 7),
                    result: data.last_push_result ?? '',
                  })
                : t('push.noPush')}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('push.limit', {
                mb: Math.round(data.max_push_bytes / (1024 * 1024)),
              })}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setConfirm('rotate')
                }}
              >
                <RefreshCwGlassIcon className="size-4" aria-hidden="true" />
                {t('push.rotate')}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setConfirm('revoke')
                }}
              >
                <BanGlassIcon className="size-4" aria-hidden="true" />
                {t('push.revoke')}
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>

      <ConfirmDialog
        open={confirm === 'rotate'}
        onOpenChange={(o) => {
          if (!o) setConfirm(null)
        }}
        title={t('push.rotateTitle')}
        description={t('push.rotateBody')}
        confirmLabel={t('push.rotate')}
        pendingLabel={t('push.rotating')}
        pending={issue.isPending}
        error={issue.isError ? issue.error.message : null}
        tone="destructive"
        onConfirm={runIssue}
      />
      <ConfirmDialog
        open={confirm === 'revoke'}
        onOpenChange={(o) => {
          if (!o) setConfirm(null)
        }}
        title={t('push.revokeTitle')}
        description={t('push.revokeBody')}
        confirmLabel={t('push.revoke')}
        pendingLabel={t('push.revoking')}
        pending={revoke.isPending}
        error={revoke.isError ? revoke.error.message : null}
        tone="destructive"
        onConfirm={() => {
          revoke.mutate(undefined, {
            onSuccess: () => {
              setConfirm(null)
              setIssued(null)
              toast.add({ title: t('push.revoked'), type: 'success' })
            },
          })
        }}
      />
    </Card>
  )
}
