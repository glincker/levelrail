import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useAppHookRuns, type HookRun } from '../queries/appHookRuns'
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'

const hooksSchema = z.object({
  preDeploy: z.string().trim(),
  postDeploy: z.string().trim(),
})

type HooksFormValues = z.infer<typeof hooksSchema>

function toFieldValues(app: AppDetail): HooksFormValues {
  return {
    preDeploy: app.hooks?.pre_deploy ?? '',
    postDeploy: app.hooks?.post_deploy ?? '',
  }
}

// Same full-replace-PUT pattern as HealthCheckEditor/DeployStrategyEditor,
// bound to AppDetail.hooks instead. Saving is only staged, not live:
// hooks run when the reconciler creates a fresh container
// (internal/reconcile/application.Controller), so
// useRestartRequiredToast's "restart to apply" messaging applies here
// exactly the way it already does for HealthCheckEditor.
export function HooksEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const notifyRestartRequired = useRestartRequiredToast()
  const hasHooks = !!(app.hooks?.pre_deploy || app.hooks?.post_deploy)
  const { data: hookRuns } = useAppHookRuns(app.name, hasHooks)

  const { register, handleSubmit } = useForm<HooksFormValues>({
    resolver: zodResolver(hooksSchema),
    values: toFieldValues(app),
    resetOptions: { keepDirtyValues: true },
  })

  const onSubmit = handleSubmit((values) => {
    const preDeploy = values.preDeploy.trim()
    const postDeploy = values.postDeploy.trim()
    updateApp.mutate(
      {
        ...app,
        hooks:
          preDeploy || postDeploy
            ? { pre_deploy: preDeploy || undefined, post_deploy: postDeploy || undefined }
            : null,
      },
      {
        onSuccess: () => {
          notifyRestartRequired(app.name, 'Deploy hooks saved.')
        },
      },
    )
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TerminalIcon className="size-4" />
          Deploy hooks
        </CardTitle>
        <CardDescription>
          Shell commands run inside the container at deploy time. A
          failing pre-deploy hook blocks cutover; a failing post-deploy
          hook is reported but does not roll back an already-healthy
          deploy.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="hooks-pre-deploy">
              Pre-deploy command
            </FieldLabel>
            <Input
              id="hooks-pre-deploy"
              {...register('preDeploy')}
              className="font-mono"
              placeholder="rails db:migrate"
            />
            <FieldDescription>
              Runs before the new container receives traffic. A nonzero
              exit code blocks the deploy.
            </FieldDescription>
            {hasHooks ? (
              <HookRunSummary label="Last pre-deploy run" run={hookRuns?.pre_deploy} />
            ) : null}
          </Field>

          <Field>
            <FieldLabel htmlFor="hooks-post-deploy">
              Post-deploy command
            </FieldLabel>
            <Input
              id="hooks-post-deploy"
              {...register('postDeploy')}
              className="font-mono"
              placeholder="curl -f https://hooks.example.com/deployed"
            />
            <FieldDescription>
              Runs once the new container is fully live. A nonzero exit
              code is reported but never undoes the deploy.
            </FieldDescription>
            {hasHooks ? (
              <HookRunSummary label="Last post-deploy run" run={hookRuns?.post_deploy} />
            ) : null}
          </Field>

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={updateApp.isPending}>
              {updateApp.isPending ? 'Saving...' : 'Save deploy hooks'}
            </Button>
          </div>
          {updateApp.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateApp.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}

function HookRunSummary({ label, run }: { label: string; run?: HookRun }) {
  if (!run) {
    return (
      <p className="mt-1 text-xs text-muted-foreground">
        {label}: never run yet.
      </p>
    )
  }
  return (
    <div className="mt-1 space-y-1">
      <p className="text-xs text-muted-foreground">
        {label}:{' '}
        <Badge variant={run.success ? 'success' : 'destructive'}>
          {run.success ? 'success' : `failed (exit ${run.exit_code})`}
        </Badge>{' '}
        at {new Date(run.ran_at).toLocaleString()}
      </p>
      {run.output ? (
        <pre className="max-h-32 overflow-auto rounded-md bg-muted p-2 text-xs">
          {run.output}
        </pre>
      ) : null}
    </div>
  )
}
