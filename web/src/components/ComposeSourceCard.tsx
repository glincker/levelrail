import { useId, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GitBranchGlassIcon,
  LoaderGlassIcon,
  RocketGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { formatAge } from '../lib/format'
import {
  composeFormFromResource,
  composePathProblem,
  composeSourceErrorKey,
  composeSourcePayload,
  emptyComposeSourceForm,
  type ComposeSourceForm,
} from '../lib/composeSourceForm'
import {
  useComposeSource,
  useDeployComposeSource,
  useDisconnectComposeSource,
  useSetComposeSource,
} from '../queries/composeSource'
import { useComposeStack } from '../queries/composeStack'
import type { ComposePhase } from '../types/compose'
import type { GitSourceTriggerMode } from '../types/gitSource'
import { GitSourceCopyField } from './GitSourceCopyField'

type BadgeVariant = 'success' | 'warning' | 'destructive' | 'muted' | 'info'

const PHASE_VARIANT: Record<ComposePhase, BadgeVariant> = {
  running: 'success',
  progressing: 'info',
  degraded: 'warning',
  failed: 'destructive',
  stopped: 'muted',
}

interface OnceSecret {
  webhookUrl: string
  secret: string
}

// Connects a compose stack to a git repository. Rendered only for compose
// apps; the webhook secret lives in component state for one view only.
export default function ComposeSourceCard({ appName }: { appName: string }) {
  const { t } = useTranslation('compose')
  const stack = useComposeStack(appName)
  const query = useComposeSource(appName)
  const save = useSetComposeSource(appName)
  const deploy = useDeployComposeSource(appName)
  const disconnect = useDisconnectComposeSource(appName)
  const ids = useId()

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<ComposeSourceForm>(emptyComposeSourceForm)
  const [once, setOnce] = useState<OnceSecret | null>(null)
  const [confirmDisconnect, setConfirmDisconnect] = useState(false)

  if (!stack.data) return null

  const notConnected =
    query.error instanceof ApiError && query.error.status === 404
  const loadError = query.error && !notConnected ? query.error : null
  const source = query.data
  const showForm = editing || (notConnected && !once)
  const pathProblem = composePathProblem(form.composePath)
  const repoMissing = form.repoUrl.trim() === ''

  function friendly(error: unknown): string {
    const key = composeSourceErrorKey(error)
    if (key) return t(`source.errors.${key}`)
    return error instanceof Error ? error.message : t('source.errors.generic')
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    if (repoMissing || pathProblem) return
    save.mutate(composeSourcePayload(form), {
      onSuccess: (resource) => {
        setEditing(false)
        setForm(emptyComposeSourceForm())
        if (resource.webhook_secret) {
          setOnce({
            webhookUrl: `${window.location.origin}${resource.webhook_url}`,
            secret: resource.webhook_secret,
          })
        } else {
          toast.add({ title: t('source.updated'), type: 'success' })
        }
      },
    })
  }

  function runDeploy() {
    deploy.mutate(undefined, {
      onSuccess: () => {
        toast.add({ title: t('source.deployStarted'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('source.deployFailed'),
          description: friendly(error),
          type: 'error',
        })
      },
    })
  }

  function runDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setConfirmDisconnect(false)
        setOnce(null)
        toast.add({ title: t('source.disconnected'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('source.disconnectFailed'),
          description: friendly(error),
          type: 'error',
        })
      },
    })
  }

  const triggerOptions: { value: GitSourceTriggerMode; label: string }[] = [
    { value: 'push', label: t('source.trigger.push') },
    { value: 'release', label: t('source.trigger.release') },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchGlassIcon className="size-5 text-muted-foreground" />
          {t('source.title')}
        </CardTitle>
        <CardDescription>{t('source.intro')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderGlassIcon className="size-5 animate-spin motion-reduce:animate-none" />
            {t('source.loading')}
          </p>
        ) : null}

        {loadError ? (
          <Alert variant="danger" className="flex items-start gap-2">
            <TriangleAlertGlassIcon className="mt-0.5 size-5 shrink-0" />
            <p className="text-sm">{friendly(loadError)}</p>
          </Alert>
        ) : null}

        {once ? (
          <div className="space-y-3">
            <Alert variant="warning" className="flex items-start gap-2">
              <TriangleAlertGlassIcon className="mt-0.5 size-5 shrink-0" />
              <p className="text-sm">{t('source.secretOnce')}</p>
            </Alert>
            <GitSourceCopyField
              id={`${ids}-webhook-url`}
              label={t('source.webhookUrl')}
              value={once.webhookUrl}
            />
            <GitSourceCopyField
              id={`${ids}-webhook-secret`}
              label={t('source.webhookSecret')}
              value={once.secret}
              description={t('source.secretHint')}
            />
            <Button type="button" size="sm" onClick={() => setOnce(null)}>
              {t('source.done')}
            </Button>
          </div>
        ) : null}

        {showForm ? (
          <form
            className="space-y-3"
            onSubmit={submit}
            aria-label={t('source.formLabel')}
            noValidate
          >
            <div className="grid gap-3 sm:grid-cols-[2fr_1fr]">
              <Field>
                <FieldLabel htmlFor={`${ids}-repo`}>
                  {t('source.repoUrl')}
                </FieldLabel>
                <Input
                  id={`${ids}-repo`}
                  value={form.repoUrl}
                  placeholder={t('source.repoPlaceholder')}
                  autoComplete="off"
                  spellCheck={false}
                  required
                  onChange={(e) =>
                    setForm({ ...form, repoUrl: e.target.value })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor={`${ids}-branch`}>
                  {t('source.branch')}
                </FieldLabel>
                <Input
                  id={`${ids}-branch`}
                  value={form.branch}
                  autoComplete="off"
                  spellCheck={false}
                  onChange={(e) => setForm({ ...form, branch: e.target.value })}
                />
              </Field>
              <Field data-invalid={pathProblem ? true : undefined}>
                <FieldLabel htmlFor={`${ids}-path`}>
                  {t('source.composePath')}
                </FieldLabel>
                <Input
                  id={`${ids}-path`}
                  value={form.composePath}
                  autoComplete="off"
                  spellCheck={false}
                  aria-invalid={pathProblem ? true : undefined}
                  aria-describedby={
                    pathProblem ? `${ids}-path-error` : undefined
                  }
                  onChange={(e) =>
                    setForm({ ...form, composePath: e.target.value })
                  }
                />
                {pathProblem ? (
                  <FieldError id={`${ids}-path-error`}>
                    {t(`source.pathErrors.${pathProblem}`)}
                  </FieldError>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor={`${ids}-token`}>
                  {t('source.token')}
                </FieldLabel>
                <Input
                  id={`${ids}-token`}
                  type="password"
                  value={form.token}
                  autoComplete="off"
                  placeholder={
                    source?.has_token ? t('source.tokenKeep') : undefined
                  }
                  onChange={(e) => setForm({ ...form, token: e.target.value })}
                />
              </Field>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              <span className="text-sm text-muted-foreground">
                {t('source.triggerLabel')}
              </span>
              <SegmentedControl
                aria-label={t('source.triggerLabel')}
                value={form.triggerMode}
                options={triggerOptions}
                onValueChange={(v) => setForm({ ...form, triggerMode: v })}
              />
            </div>
            {save.error ? (
              <Alert
                variant="danger"
                role="alert"
                className="flex items-start gap-2"
              >
                <TriangleAlertGlassIcon className="mt-0.5 size-5 shrink-0" />
                <p className="text-sm">{friendly(save.error)}</p>
              </Alert>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <Button
                type="submit"
                size="sm"
                disabled={save.isPending || repoMissing || pathProblem !== null}
                aria-busy={save.isPending || undefined}
              >
                {save.isPending ? (
                  <LoaderGlassIcon className="size-5 animate-spin motion-reduce:animate-none" />
                ) : null}
                {source ? t('source.save') : t('source.connect')}
              </Button>
              {editing ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setEditing(false)
                    save.reset()
                  }}
                >
                  {t('source.cancel')}
                </Button>
              ) : null}
            </div>
          </form>
        ) : null}

        {source && !editing ? (
          <div className="space-y-3">
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
              <dt className="text-muted-foreground">{t('source.repoUrl')}</dt>
              <dd className="min-w-0 truncate font-mono">{source.repo_url}</dd>
              <dt className="text-muted-foreground">{t('source.branch')}</dt>
              <dd className="min-w-0 truncate font-mono">{source.branch}</dd>
              <dt className="text-muted-foreground">
                {t('source.composePath')}
              </dt>
              <dd className="min-w-0 truncate font-mono">
                {source.compose_path}
              </dd>
              <dt className="text-muted-foreground">
                {t('source.triggerLabel')}
              </dt>
              <dd>
                {source.trigger_mode === 'release'
                  ? t('source.trigger.release')
                  : t('source.trigger.push')}
              </dd>
              <dt className="text-muted-foreground">{t('source.state')}</dt>
              <dd className="flex flex-wrap items-center gap-2">
                <Badge variant={PHASE_VARIANT[stack.data.phase]}>
                  {t(`stack.phase.${stack.data.phase}`)}
                </Badge>
                <span className="text-muted-foreground">
                  {stack.data.source === 'git'
                    ? t('source.fromGit')
                    : t('source.notFromGit')}
                  {stack.data.deployed_at
                    ? ` ${t('stack.deployedAt', { when: formatAge(stack.data.deployed_at) })}`
                    : ''}
                </span>
              </dd>
            </dl>
            {once ? null : (
              <GitSourceCopyField
                id={`${ids}-webhook-url-connected`}
                label={t('source.webhookUrl')}
                value={`${window.location.origin}${source.webhook_url}`}
              />
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                size="sm"
                disabled={deploy.isPending}
                aria-busy={deploy.isPending || undefined}
                onClick={runDeploy}
              >
                {deploy.isPending ? (
                  <LoaderGlassIcon className="size-5 animate-spin motion-reduce:animate-none" />
                ) : (
                  <RocketGlassIcon className="size-5" />
                )}
                {t('source.deployNow')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setOnce(null)
                  setForm(composeFormFromResource(source))
                  setEditing(true)
                }}
              >
                {t('source.edit')}
              </Button>
              {confirmDisconnect ? (
                <>
                  <Button
                    type="button"
                    size="sm"
                    variant="destructive"
                    disabled={disconnect.isPending}
                    aria-busy={disconnect.isPending || undefined}
                    onClick={runDisconnect}
                  >
                    {disconnect.isPending ? (
                      <LoaderGlassIcon className="size-5 animate-spin motion-reduce:animate-none" />
                    ) : null}
                    {t('source.confirmDisconnect')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setConfirmDisconnect(false)}
                  >
                    {t('source.cancel')}
                  </Button>
                </>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setConfirmDisconnect(true)}
                >
                  {t('source.disconnect')}
                </Button>
              )}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
