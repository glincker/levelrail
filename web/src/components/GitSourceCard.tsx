import { useState } from 'react'
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
  incompleteRowsMessage,
  type FormState,
} from '../lib/gitSourceForm'
import { connectGitSource } from '../lib/gitSourceConnect'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceResource } from '../types/gitSource'
import { GitSourceConnectedView } from './GitSourceConnectedView'
import { GitSourceEditForm } from './GitSourceEditForm'
import { GitSourceJustConnected } from './GitSourceJustConnected'

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
  const query = useGitSource(app.name)
  const queryClient = useQueryClient()
  const connectMutation = useMutation({
    mutationFn: ({ appName, form }: { appName: string; form: FormState }) =>
      connectGitSource(appName, form),
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
    if (form.fanoutMode === 'services') {
      if (!form.services.some((row) => row.serviceName.trim())) {
        return 'Add at least one service, or switch back to single service.'
      }
      const incomplete = incompleteRowIndexes(form.services)
      return incomplete.length > 0 ? incompleteRowsMessage(incomplete) : null
    }
    const incomplete = incompleteRowIndexes(form.additionalServices)
    return incomplete.length > 0 ? incompleteRowsMessage(incomplete) : null
  }

  function submit() {
    if (!form.repoUrl.trim()) {
      toast.add({
        title: 'Pick a repository or paste a URL first.',
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
            toast.add({ title: 'Git source updated.', type: 'success' })
          }
        },
        onError: (error) => {
          toast.add({
            title: 'Could not save git source.',
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
        toast.add({ title: 'Git source disconnected.', type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not disconnect git source.',
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
          Git source
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderGlassIcon className="size-5 animate-spin" />
            Loading...
          </p>
        ) : null}

        {otherError ? (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-destructive">
            <TriangleAlertGlassIcon className="mt-0.5 size-6 shrink-0 text-tone-warning" />
            <p className="text-sm">{otherError.message}</p>
          </div>
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
        ) : query.data ? (
          <GitSourceConnectedView
            app={app}
            source={query.data}
            disconnecting={deleteGitSource.isPending}
            onEdit={() => {
              if (query.data) startEdit(formFromResource(query.data))
            }}
            onDisconnect={disconnect}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
