import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { RocketGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import {
  GitRepoSourcePicker,
  type GitRepoSourceValue,
} from './GitRepoSourcePicker'
import { connectPickedRepo } from './deploy/connectSource'
import {
  useDeployGitSource,
  useGitSourcePlan,
  type GitSourcePlan,
} from '../queries/gitSourceConnect'
import {
  isFinalDeployStatus,
  useFirstDeployResult,
} from '../queries/gitSourceFirstDeploy'

const FIRST_DEPLOY_CLOCK_MARGIN_MS = 60_000

type Step = 'source' | 'review' | 'finish'

function connectionOf(value: GitRepoSourceValue | null): string {
  const ref = value?.providerRef
  return ref && ref.kind !== 'github' ? ref.connectionId : ''
}

function PlanReview({
  plan,
  target,
  onTarget,
}: {
  plan: GitSourcePlan
  target: string
  onTarget: (name: string) => void
}) {
  const { t } = useTranslation('gitSource')
  const p = plan.preserved
  return (
    <div className="space-y-3 text-sm">
      {plan.group.length > 0 && (
        <fieldset className="space-y-2">
          <legend className="font-medium">{t('dialog.pickService')}</legend>
          {[plan.app, ...plan.group.map((m) => m.name)].map((n) => (
            <label key={n} className="flex items-center gap-2">
              <input
                type="radio"
                name="git-source-target"
                checked={target === n}
                onChange={() => onTarget(n)}
              />
              <span>{n}</span>
              {n !== plan.app && (
                <span className="text-muted-foreground">
                  {t(
                    `dialog.effect.${plan.group.find((m) => m.name === n)?.effect ?? 'unchanged'}`,
                  )}
                </span>
              )}
            </label>
          ))}
        </fieldset>
      )}
      {plan.suggestion && (
        <p>
          {t('dialog.detected', {
            build: plan.suggestion.framework ?? plan.suggestion.build_type,
          })}
        </p>
      )}
      {plan.detect_error && (
        <p className="text-muted-foreground">
          {t(`dialog.detectError.${plan.detect_error}`)}
        </p>
      )}
      <p className="text-muted-foreground">
        {t('dialog.kept', {
          env: p.env_vars + p.secret_env,
          domains: p.domains.length,
          volumes: p.volumes.length,
        })}
      </p>
      {plan.warnings.includes('connection_needs_read_sensitive') && (
        <p className="text-muted-foreground">
          {t('dialog.connectionNeedsRead')}
        </p>
      )}
      {plan.warnings.includes('protected_environment') && (
        <p className="text-muted-foreground">{t('dialog.protected')}</p>
      )}
    </div>
  )
}

function FirstDeployResult({
  appName,
  since,
}: {
  appName: string
  since: string
}) {
  const { t } = useTranslation('gitSource')
  const d = useFirstDeployResult(appName, since)
  const status = d?.status ?? 'queued'
  return (
    <div className="space-y-2 text-sm" aria-live="polite">
      <p className="font-medium">{t(`dialog.status.${status}`, status)}</p>
      {d?.steps?.failing_step && (
        <p className="text-muted-foreground">
          {t('dialog.failingStep', { step: d.steps.failing_step })}
        </p>
      )}
      {d?.error_summary && (
        <pre className="max-h-40 overflow-auto rounded-lg bg-muted p-2 text-xs">
          {d.error_summary}
        </pre>
      )}
      {(!d || !isFinalDeployStatus(status)) && (
        <p className="text-muted-foreground">{t('connect.deployRunning')}</p>
      )}
      <Link
        to="/apps/$name/deploys"
        params={{ name: appName }}
        className="text-primary underline-offset-4 hover:underline"
      >
        {t('connect.viewDeploy')}
      </Link>
    </div>
  )
}

// Guided connect: pick a repo, review what is detected and kept, then
// deploy now or link only. Every write goes through the existing endpoints.
export function GitSourceConnectDialog({
  appName,
  open,
  onOpenChange,
}: {
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('gitSource')
  const [step, setStep] = useState<Step>('source')
  const [value, setValue] = useState<GitRepoSourceValue | null>(null)
  const [target, setTarget] = useState(appName)
  const [busy, setBusy] = useState(false)
  const [since, setSince] = useState<string | null>(null)
  const repoUrl = value?.repoUrl.trim() ?? ''
  const branch = value?.branch.trim() ?? ''
  // The plan follows the picked service: its kept config and siblings differ.
  const plan = useGitSourcePlan(
    target,
    repoUrl,
    branch,
    step !== 'source' && repoUrl !== '',
    connectionOf(value),
  )
  const deploy = useDeployGitSource(target)

  async function finish(deployNow: boolean) {
    if (!value) return
    setBusy(true)
    try {
      const sg = plan.data?.suggestion
      const gs = await connectPickedRepo(
        target,
        value,
        sg && {
          build_type: sg.build_type,
          build_path: sg.build_path,
          base_directory: sg.base_directory,
        },
      )
      if (deployNow) {
        // Margin for browser clock skew against the server's started_at.
        const startedAt = new Date(
          Date.now() - FIRST_DEPLOY_CLOCK_MARGIN_MS,
        ).toISOString()
        await deploy.mutateAsync(gs.branch)
        setSince(startedAt)
        setStep('finish')
      } else {
        toast.add({ title: t('dialog.linked'), type: 'success' })
        onOpenChange(false)
      }
    } catch (error) {
      toast.add({
        title: t('connect.deployFailed'),
        description: error instanceof Error ? error.message : undefined,
        type: 'error',
      })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('dialog.title')}</DialogTitle>
          <DialogDescription>{t(`dialog.step.${step}`)}</DialogDescription>
        </DialogHeader>
        {step === 'source' && (
          <GitRepoSourcePicker disabled={busy} onSelect={setValue} />
        )}
        {step === 'review' &&
          (plan.data ? (
            <PlanReview plan={plan.data} target={target} onTarget={setTarget} />
          ) : (
            <p className="text-sm text-muted-foreground">
              {plan.error ? plan.error.message : t('dialog.reading')}
            </p>
          ))}
        {step === 'finish' && since && (
          <FirstDeployResult appName={target} since={since} />
        )}
        <DialogFooter>
          {step === 'source' && (
            <Button
              type="button"
              disabled={repoUrl === ''}
              onClick={() => setStep('review')}
            >
              {t('dialog.next')}
            </Button>
          )}
          {step === 'review' && (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep('source')}
              >
                {t('dialog.back')}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={busy || !plan.data}
                onClick={() => void finish(false)}
              >
                {t('dialog.linkOnly')}
              </Button>
              <Button
                type="button"
                disabled={
                  busy ||
                  !plan.data ||
                  plan.data.warnings.includes('protected_environment') ||
                  plan.data.warnings.includes('protection_unknown')
                }
                onClick={() => void finish(true)}
              >
                <RocketGlassIcon className="size-4" />
                {t('connect.deployNow')}
              </Button>
            </>
          )}
          {step === 'finish' && (
            <Button type="button" onClick={() => onOpenChange(false)}>
              {t('dialog.done')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
