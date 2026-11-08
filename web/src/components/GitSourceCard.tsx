import { useState } from 'react'
import { Alert } from '@/components/ui/alert'
import { useTranslation } from 'react-i18next'
import {
  GitBranchGlassIcon,
  LoaderGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  gitSourceKeys,
  useDeleteGitSource,
  useGitSource,
} from '../queries/gitSources'
import { ApiError } from '../lib/apiError'
import {
  emptyForm,
  formFromResource,
  incompleteRowIndexes,
  incompleteRowNumbers,
  type FormState,
} from '../lib/gitSourceForm'
import { connectGitSource } from '../lib/gitSourceConnect'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceResource } from '../types/gitSource'
import { GitSourceConnectedView } from './GitSourceConnectedView'
import { GitSourceEditForm } from './GitSourceEditForm'
import { GitSourceJustConnected } from './GitSourceJustConnected'
import { GitSourceSwitchPlan } from './GitSourceSwitchPlan'
import { GitSourceDeployNow } from './GitSourceDeployNow'
import { setGitBuild } from '../queries/gitBuildSettings'
import type { GitSourcePlanSuggestion } from '../queries/gitSourceConnect'

// Git source connect/manage card, PUT/GET/DELETE
// /api/v1/apps/{name}/git-source (internal/api/git_sources.go): the
// missing UI half of that endpoint's own deferred follow-up. Rendered
// from routes/apps/$name/overview.tsx, the same section route
// DeployStrategyEditor already lives on, mirroring
// DatabasePublicAccessCard.tsx's Card shape (a toggle-style connect/
// disconnect action plus a warning banner once connected) and
// AddNodeDialog.tsx's "mint a secret, show it exactly once" pattern for
// the generated webhook secret specifically: it never comes back from
// GET (gitSourceResource's own doc comment, internal/api/git_sources.go),
// so this component's only chance to show it is the connect response the
// instant a repo is first connected.
//
// Repo/branch selection is GitRepoSourcePicker (also used by
// CreateAppFromGitFields.tsx's app-creation wizard): this card no longer
// hand-rolls its own URL/branch inputs, so picking a connected
// GitHub/GitLab/Bitbucket repo here registers a push webhook automatically
// the same way the wizard does, instead of only ever producing a manual,
// paste-the-webhook-by-hand connection.
export function GitSourceCard({ app }: { app: AppDetail }) {
  const { t } = useTranslation('gitSource')
  const query = useGitSource(app.name)
  const queryClient = useQueryClient()
  // Static build command and publish dir from the plan; the connect body has no
  // field for them, so they are saved right after the link.
  const [staticBuild, setStaticBuild] =
    useState<GitSourcePlanSuggestion | null>(null)
  const connectMutation = useMutation({
    mutationFn: async ({
      appName,
      form,
    }: {
      appName: string
      form: FormState
    }) => {
      const result = await connectGitSource(appName, form)
      if (staticBuild && form.buildType === 'static') {
        await setGitBuild(appName, {
          build_type: 'static',
          build_path: form.buildPath,
          base_directory: form.baseDirectory,
          build_command: staticBuild.build_command,
          publish_dir: staticBuild.publish_dir,
        })
      }
      return result
    },
    onSuccess: (result) => {
      queryClient.setQueryData(gitSourceKeys.detail(app.name), result.resource)
    },
  })
  const deleteGitSource = useDeleteGitSource(app.name)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<FormState>(emptyForm)
  // The one and only place the generated webhook secret is ever held:
  // it never comes back from GET, so it is not re-derived from query.data.
  const [justConnected, setJustConnected] = useState<GitSourceResource | null>(
    null,
  )
  const [webhookAutoRegistered, setWebhookAutoRegistered] = useState(false)
  const [webhookError, setWebhookError] = useState<string | undefined>(
    undefined,
  )
  const [rowsError, setRowsError] = useState<string | null>(null)

  const notConnected =
    query.error instanceof ApiError && query.error.status === 404
  const otherError = query.error && !notConnected ? query.error : null

  function startEdit(prefill: FormState) {
    setForm(prefill)
    setEditing(true)
    setJustConnected(null)
    setRowsError(null)
  }

  function cancelEdit() {
    setEditing(false)
    setForm(emptyForm())
    setRowsError(null)
  }

  // Returns the first problem with the fan-out rows, or null when they are fine.
  function rowsProblem(): string | null {
    const missingNames = (incomplete: number[]) =>
      incomplete.length > 0
        ? t('card.rowsMissingName', {
            count: incomplete.length,
            rows: incompleteRowNumbers(incomplete),
          })
        : null
    if (form.fanoutMode === 'services') {
      if (!form.services.some((row) => row.serviceName.trim())) {
        return t('card.needService')
      }
      return missingNames(incompleteRowIndexes(form.services))
    }
    return missingNames(incompleteRowIndexes(form.additionalServices))
  }

  function submit() {
    if (!form.repoUrl.trim()) {
      toast.add({
        title: t('card.pickRepoFirst'),
        type: 'error',
      })
      return
    }
    const problem = rowsProblem()
    if (problem) {
      setRowsError(problem)
      return
    }
    setRowsError(null)
    connectMutation.mutate(
      { appName: app.name, form },
      {
        onSuccess: (result) => {
          setEditing(false)
          setForm(emptyForm())
          if (result.resource.webhook_secret) {
            setJustConnected(result.resource)
            setWebhookAutoRegistered(result.autoRegistered)
            setWebhookError(result.webhookError)
          } else {
            toast.add({ title: t('card.updated'), type: 'success' })
          }
        },
        onError: (error) => {
          toast.add({
            title: t('card.saveFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function disconnect() {
    deleteGitSource.mutate(undefined, {
      onSuccess: () => {
        setJustConnected(null)
        toast.add({ title: t('card.disconnected'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('card.disconnectFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchGlassIcon className="size-5 text-muted-foreground" />
          {notConnected ? t('connect.title') : t('card.title')}
        </CardTitle>
        {notConnected ? (
          <p className="text-sm text-muted-foreground">{t('connect.intro')}</p>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderGlassIcon className="size-5 animate-spin" />
            {t('card.loading')}
          </p>
        ) : null}

        {otherError ? (
          <Alert variant="danger" className="flex items-start gap-2">
            <TriangleAlertGlassIcon className="mt-0.5 size-6 shrink-0 text-tone-warning" />
            <p className="text-sm">{otherError.message}</p>
          </Alert>
        ) : null}

        {justConnected ? (
          <GitSourceJustConnected
            resource={justConnected}
            autoRegistered={webhookAutoRegistered}
            webhookError={webhookError}
            onDone={() => {
              setJustConnected(null)
            }}
          />
        ) : editing || notConnected ? (
          <>
            {notConnected ? (
              <GitSourceSwitchPlan
                appName={app.name}
                repoUrl={form.repoUrl}
                branch={form.branch}
                onApply={(s) => {
                  setForm({
                    ...form,
                    buildType: s.build_type,
                    buildPath: s.build_path ?? '',
                    baseDirectory: s.base_directory ?? '',
                  })
                  setStaticBuild(s.build_type === 'static' ? s : null)
                }}
              />
            ) : null}
            <GitSourceEditForm
              appName={app.name}
              form={form}
              setForm={setForm}
              error={rowsError}
              setError={setRowsError}
              pending={connectMutation.isPending}
              notConnected={notConnected}
              onSubmit={submit}
              onCancel={cancelEdit}
            />
          </>
        ) : query.data ? (
          <>
            <GitSourceConnectedView
              app={app}
              source={query.data}
              disconnecting={deleteGitSource.isPending}
              onEdit={() => {
                if (query.data) startEdit(formFromResource(query.data))
              }}
              onDisconnect={disconnect}
            />
            <GitSourceDeployNow appName={app.name} branch={query.data.branch} />
          </>
        ) : null}
        {justConnected ? (
          <GitSourceDeployNow
            appName={app.name}
            branch={justConnected.branch}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
