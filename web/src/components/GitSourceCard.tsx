import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  GitBranchIcon,
  PlusIcon,
  SpinnerIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import { BrandIcon, type BrandIconName } from '@/components/BrandIcon'
import {
  gitSourceKeys,
  setGitSource,
  useDeleteGitSource,
  useGitSource,
} from '../queries/gitSources'
import { connectGitHubRepoAsSource } from '../queries/githubApp'
import { connectGitLabProjectAsSource } from '../queries/gitlabApp'
import { connectBitbucketRepoAsSource } from '../queries/bitbucketApp'
import { GitRepoSourcePicker, type GitRepoSourceValue } from './GitRepoSourcePicker'
import { ApiError } from '../lib/apiError'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceBuild, GitSourceBuildType, GitSourceResource } from '../types/gitSource'

// Git source connect/manage card, PUT/GET/DELETE
// /api/v1/apps/{name}/git-source (internal/api/git_sources.go): the
// missing UI half of TASKS.md 1.7's own deferred follow-up. Rendered
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
// CreateAppFromGitFields.tsx's app-creation wizard, see docs-local/
// research/git-provider-connect-ux-unification-proposal.md section 4):
// this card no longer hand-rolls its own URL/branch inputs, so picking a
// connected GitHub/GitLab/Bitbucket repo here registers a push webhook
// automatically the same way the wizard does, instead of only ever
// producing a manual, paste-the-webhook-by-hand connection.
const BUILD_PACKS: { value: GitSourceBuildType; label: string }[] = [
  { value: 'railpack', label: 'Auto-detect (Railpack)' },
  { value: 'dockerfile', label: 'Dockerfile' },
  { value: 'static', label: 'Static site' },
]

// Frameworks/languages the auto-detect build pack actually supports,
// shown next to the "Auto-detect" tab so this is a guided picker rather
// than a dropdown asking the operator to already know what Railpack is.
// Mirrors internal/build/railpack.go's supportedRailpackProviders
// exactly: node covers React and Next.js (both are the node provider,
// no separate entry), golang is go, and java covers Spring Boot, the
// framework this list exists to name.
const AUTO_DETECT_STACKS: { icon: BrandIconName; label: string }[] = [
  { icon: 'node', label: 'Node.js (React, Next.js, ...)' },
  { icon: 'go', label: 'Go' },
  { icon: 'java', label: 'Java (Spring Boot)' },
]

// AdditionalServiceRow is one editable row of FormState.additionalServices:
// an array, not a Record, so a service name can be edited mid-typing
// without colliding with another row that momentarily shares the same
// (incomplete) key.
interface AdditionalServiceRow {
  serviceName: string
  buildType: GitSourceBuildType
  buildPath: string
}

interface FormState {
  repoUrl: string
  branch: string
  buildType: GitSourceBuildType
  buildPath: string
  token: string
  // Set only by a fresh GitRepoSourcePicker pick from a connected
  // provider row, cleared on every startEdit/cancelEdit: see
  // connectGitSource's own doc comment for how this decides which
  // endpoint submit() calls.
  providerRef?: GitRepoSourceValue['providerRef']
  additionalServices: AdditionalServiceRow[]
}

// Auto-detect is the default and recommended choice: most repos need no
// build configuration at all beyond picking their language, so a blank
// connect form should not default to the manual Dockerfile path.
function emptyForm(): FormState {
  return {
    repoUrl: '',
    branch: '',
    buildType: 'railpack',
    buildPath: '',
    token: '',
    providerRef: undefined,
    additionalServices: [],
  }
}

function formFromResource(g: GitSourceResource): FormState {
  return {
    repoUrl: g.repo_url,
    branch: g.branch,
    buildType: g.build_type,
    buildPath: g.build_path ?? '',
    token: '',
    providerRef: undefined,
    additionalServices: Object.entries(g.additional_services ?? {}).map(([serviceName, build]) => ({
      serviceName,
      buildType: build.build_type,
      buildPath: build.build_path ?? '',
    })),
  }
}

// A row only counts as "incomplete" once the operator has actually put
// something into it: a brand new row's buildType already defaults to
// 'dockerfile' with an empty buildPath, so an untouched blank row is not
// flagged, only one where buildPath or buildType diverges from that
// default while the name is still empty.
function incompleteRowIndexes(rows: AdditionalServiceRow[]): number[] {
  return rows
    .map((row, index) => ({ row, index }))
    .filter(({ row }) => !row.serviceName.trim() && (row.buildPath.trim() !== '' || row.buildType !== 'dockerfile'))
    .map(({ index }) => index)
}

function additionalServicesPayload(rows: AdditionalServiceRow[]): Record<string, GitSourceBuild> | undefined {
  const entries = rows
    .filter((row) => row.serviceName.trim())
    .map((row): [string, GitSourceBuild] => [
      row.serviceName.trim(),
      { build_type: row.buildType, build_path: row.buildPath.trim() || undefined },
    ])
  return entries.length > 0 ? Object.fromEntries(entries) : undefined
}

interface ConnectGitSourceResult {
  resource: GitSourceResource
  autoRegistered: boolean
  webhookError?: string
}

// connectGitSource mirrors CreateAppFromGitFields.tsx's own
// connectGitSourceFor: a fresh provider pick (form.providerRef) uses that
// provider's use-as-source endpoint, which registers a push webhook
// automatically, otherwise it falls back to the generic PUT
// .../git-source endpoint this card always used before. A provider pick
// combined with additional services also falls back: none of the three
// use-as-source endpoints accept additional_services (only the generic
// endpoint does), so a monorepo fan-out connect always takes the manual
// path, trading away auto webhook registration for that one case.
async function connectGitSource(
  appName: string,
  form: FormState,
): Promise<ConnectGitSourceResult> {
  const branch = form.branch.trim() || undefined
  const buildPath = form.buildPath.trim() || undefined
  const additionalServices = additionalServicesPayload(form.additionalServices)

  if (form.providerRef && !additionalServices) {
    if (form.providerRef.kind === 'github') {
      const { webhook_registered: autoRegistered, webhook_error: webhookError, ...resource } =
        await connectGitHubRepoAsSource(form.providerRef.owner, form.providerRef.repo, {
          app_name: appName,
          branch,
          build_type: form.buildType,
          build_path: buildPath,
        })
      return { resource, autoRegistered, webhookError }
    }
    if (form.providerRef.kind === 'gitlab') {
      const resource = await connectGitLabProjectAsSource(form.providerRef.projectId, {
        app_name: appName,
        branch,
        build_type: form.buildType,
        build_path: buildPath,
      })
      return { resource, autoRegistered: true }
    }
    const resource = await connectBitbucketRepoAsSource(
      form.providerRef.workspace,
      form.providerRef.repoSlug,
      { app_name: appName, branch, build_type: form.buildType, build_path: buildPath },
    )
    return { resource, autoRegistered: true }
  }

  const resource = await setGitSource(appName, {
    repo_url: form.repoUrl.trim(),
    branch,
    build_type: form.buildType,
    build_path: buildPath,
    token: form.token.trim() || undefined,
    additional_services: additionalServices,
  })
  return { resource, autoRegistered: false }
}

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
  // set from connectGitSource's own result (only present on the create
  // path, gitSourceResource's own doc comment), cleared the moment this
  // card unmounts a connect/edit form. Never re-derived from query.data,
  // which never carries it.
  const [justConnected, setJustConnected] = useState<GitSourceResource | null>(null)
  const [webhookAutoRegistered, setWebhookAutoRegistered] = useState(false)
  const [webhookError, setWebhookError] = useState<string | undefined>(undefined)
  const [secretCopied, setSecretCopied] = useState(false)
  const [urlCopied, setUrlCopied] = useState(false)
  const [additionalServicesError, setAdditionalServicesError] = useState<string | null>(null)

  const notConnected = query.error instanceof ApiError && query.error.status === 404
  const otherError = query.error && !notConnected ? query.error : null

  function startEdit(prefill: FormState) {
    setForm(prefill)
    setEditing(true)
    setJustConnected(null)
    setAdditionalServicesError(null)
  }

  function cancelEdit() {
    setEditing(false)
    setForm(emptyForm())
    setAdditionalServicesError(null)
  }

  function submit() {
    if (!form.repoUrl.trim()) {
      toast.add({ title: 'Pick a repository or paste a URL first.', type: 'error' })
      return
    }
    const incomplete = incompleteRowIndexes(form.additionalServices)
    if (incomplete.length > 0) {
      const rowLabel = incomplete.length === 1 ? 'Row' : 'Rows'
      const rowNumbers = incomplete.map((index) => index + 1).join(', ')
      setAdditionalServicesError(
        `${rowLabel} ${rowNumbers} ${incomplete.length === 1 ? 'is' : 'are'} missing a service name.`,
      )
      return
    }
    setAdditionalServicesError(null)
    connectMutation.mutate(
      { appName: app.name, form },
      {
        onSuccess: (result) => {
          setEditing(false)
          setForm(emptyForm())
          setSecretCopied(false)
          setUrlCopied(false)
          if (result.resource.webhook_secret) {
            setJustConnected(result.resource)
            setWebhookAutoRegistered(result.autoRegistered)
            setWebhookError(result.webhookError)
          } else {
            toast.add({ title: 'Git source updated.', type: 'success' })
          }
        },
        onError: (error) => {
          toast.add({ title: 'Could not save git source.', description: error.message, type: 'error' })
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
        toast.add({ title: 'Could not disconnect git source.', description: error.message, type: 'error' })
      },
    })
  }

  function copyText(text: string, mark: (v: boolean) => void) {
    void navigator.clipboard.writeText(text).then(() => {
      mark(true)
    })
  }

  const webhookFullURL = (path: string) => `${window.location.origin}${path}`

  function addAdditionalServiceRow() {
    setForm({
      ...form,
      additionalServices: [...form.additionalServices, { serviceName: '', buildType: 'dockerfile', buildPath: '' }],
    })
  }

  function updateAdditionalServiceRow(index: number, patch: Partial<AdditionalServiceRow>) {
    setForm({
      ...form,
      additionalServices: form.additionalServices.map((row, i) => (i === index ? { ...row, ...patch } : row)),
    })
    setAdditionalServicesError(null)
  }

  function removeAdditionalServiceRow(index: number) {
    setForm({
      ...form,
      additionalServices: form.additionalServices.filter((_, i) => i !== index),
    })
    setAdditionalServicesError(null)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GitBranchIcon className="size-4 text-muted-foreground" />
          Git source
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <SpinnerIcon className="size-4 animate-spin" />
            Loading...
          </p>
        ) : null}

        {otherError ? (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-destructive">
            <WarningIcon className="mt-0.5 size-4 shrink-0" />
            <p className="text-sm">{otherError.message}</p>
          </div>
        ) : null}

        {justConnected ? (
          <div className="space-y-3">
            {webhookAutoRegistered ? (
              <div className="flex items-start gap-2 rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 text-emerald-900 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-200">
                <CheckIcon className="mt-0.5 size-4 shrink-0" />
                <p className="text-sm">
                  Git source connected. A push webhook was registered
                  automatically, no manual setup needed.
                </p>
              </div>
            ) : (
              <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                <WarningIcon className="mt-0.5 size-4 shrink-0" />
                <p className="text-sm">
                  {webhookError ? (
                    webhookError
                  ) : (
                    <>
                      This webhook secret will not be shown again. Paste both
                      values into the repository&apos;s webhook settings now.
                    </>
                  )}
                </p>
              </div>
            )}

            {!webhookAutoRegistered ? (
              <>
                <Field>
                  <FieldLabel htmlFor="git-source-webhook-url">Webhook URL</FieldLabel>
                  <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
                    <code id="git-source-webhook-url" className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                      {webhookFullURL(justConnected.webhook_url)}
                    </code>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        copyText(webhookFullURL(justConnected.webhook_url), setUrlCopied)
                      }}
                    >
                      {urlCopied ? <CheckIcon /> : <CopyIcon />}
                      {urlCopied ? 'Copied' : 'Copy'}
                    </Button>
                  </div>
                </Field>

                <Field>
                  <FieldLabel htmlFor="git-source-webhook-secret">Webhook secret</FieldLabel>
                  <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
                    <code id="git-source-webhook-secret" className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                      {justConnected.webhook_secret}
                    </code>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        if (justConnected.webhook_secret) {
                          copyText(justConnected.webhook_secret, setSecretCopied)
                        }
                      }}
                    >
                      {secretCopied ? <CheckIcon /> : <CopyIcon />}
                      {secretCopied ? 'Copied' : 'Copy'}
                    </Button>
                  </div>
                  <FieldDescription>
                    Set this as the secret on a GitHub &quot;Push&quot; webhook pointed at
                    the URL above, content type application/json.
                  </FieldDescription>
                </Field>
              </>
            ) : null}

            <Button type="button" size="sm" onClick={() => { setJustConnected(null) }}>
              Done
            </Button>
          </div>
        ) : editing || notConnected ? (
          <div className="space-y-4">
            {!notConnected ? (
              <p className="text-sm text-muted-foreground">
                Currently connected to <span className="font-mono">{form.repoUrl}</span>{' '}
                @ <span className="font-mono">{form.branch}</span>. Pick a different
                repository below to re-point it, or leave it as is to only update the
                build settings.
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">
                Connect a repository so a push to its target branch auto-deploys
                this app.
              </p>
            )}

            <GitRepoSourcePicker
              disabled={connectMutation.isPending}
              onSelect={(value) => {
                setForm((current) => ({
                  ...current,
                  repoUrl: value.repoUrl,
                  branch: value.branch,
                  token: value.token ?? '',
                  providerRef: value.providerRef,
                }))
              }}
            />

            <Field>
              <FieldLabel htmlFor="git-source-build-type">Build pack</FieldLabel>
              <Tabs
                value={form.buildType}
                onValueChange={(v: unknown) => {
                  if (v === 'railpack' || v === 'dockerfile' || v === 'static') {
                    setForm({ ...form, buildType: v })
                  }
                }}
              >
                <TabsList id="git-source-build-type" className="grid w-full grid-cols-3">
                  <TabsTrigger value="railpack" disabled={connectMutation.isPending}>
                    Auto-detect
                  </TabsTrigger>
                  <TabsTrigger value="dockerfile" disabled={connectMutation.isPending}>
                    Dockerfile
                  </TabsTrigger>
                  <TabsTrigger value="static" disabled={connectMutation.isPending}>
                    Static site
                  </TabsTrigger>
                </TabsList>

                <TabsContent value="railpack" className="space-y-2 pt-2">
                  <FieldDescription>
                    Recommended. Levelrail detects your framework from the
                    repository and builds it automatically, no Dockerfile
                    needed.
                  </FieldDescription>
                  <div className="flex flex-wrap gap-2">
                    {AUTO_DETECT_STACKS.map((stack) => (
                      <span
                        key={stack.icon}
                        className="inline-flex items-center gap-1.5 rounded-md border border-input bg-muted/50 px-2 py-1 text-xs text-muted-foreground"
                      >
                        <BrandIcon name={stack.icon} className="size-3.5" />
                        {stack.label}
                      </span>
                    ))}
                  </div>
                </TabsContent>

                <TabsContent value="dockerfile" className="pt-2">
                  <Field>
                    <FieldLabel htmlFor="git-source-build-path">
                      Dockerfile path (optional)
                    </FieldLabel>
                    <Input
                      id="git-source-build-path"
                      className="font-mono"
                      placeholder="./Dockerfile"
                      autoComplete="off"
                      spellCheck={false}
                      value={form.buildPath}
                      onChange={(e) => { setForm({ ...form, buildPath: e.target.value }) }}
                      disabled={connectMutation.isPending}
                    />
                  </Field>
                </TabsContent>

                <TabsContent value="static" className="pt-2">
                  <FieldDescription>
                    Served directly by the embedded Caddy ingress, no
                    container.
                  </FieldDescription>
                </TabsContent>
              </Tabs>
            </Field>

            <Field>
              <FieldLabel>Additional services (optional)</FieldLabel>
              <FieldDescription>
                Also rebuild these sibling services from the same push, for a
                monorepo with more than one service. Each needs its own
                already-existing service name and build config. Saving with
                any row filled in always uses the direct git-source
                connection, even for a picked provider repo, since automatic
                webhook registration doesn&apos;t support monorepo fan-out yet.
              </FieldDescription>
              <div className="space-y-2">
                {form.additionalServices.map((row, index) => (
                  <div key={index} className="flex items-start gap-2">
                    <Input
                      className="font-mono"
                      placeholder="worker"
                      autoComplete="off"
                      spellCheck={false}
                      value={row.serviceName}
                      onChange={(e) => { updateAdditionalServiceRow(index, { serviceName: e.target.value }) }}
                      disabled={connectMutation.isPending}
                    />
                    <Select
                      value={row.buildType}
                      onValueChange={(v) => {
                        if (v === 'railpack' || v === 'dockerfile' || v === 'static') {
                          updateAdditionalServiceRow(index, { buildType: v })
                        }
                      }}
                    >
                      <SelectTrigger className="w-40 shrink-0">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {BUILD_PACKS.map((pack) => (
                          <SelectItem key={pack.value} value={pack.value}>
                            {pack.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Input
                      className="font-mono"
                      placeholder="./worker/Dockerfile"
                      autoComplete="off"
                      spellCheck={false}
                      value={row.buildPath}
                      onChange={(e) => { updateAdditionalServiceRow(index, { buildPath: e.target.value }) }}
                      disabled={connectMutation.isPending || row.buildType === 'railpack'}
                    />
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={connectMutation.isPending}
                      onClick={() => { removeAdditionalServiceRow(index) }}
                      aria-label="Remove additional service"
                    >
                      <TrashIcon />
                    </Button>
                  </div>
                ))}
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={connectMutation.isPending}
                  onClick={addAdditionalServiceRow}
                >
                  <PlusIcon />
                  Add service
                </Button>
              </div>
              <FieldError
                errors={additionalServicesError ? [{ message: additionalServicesError }] : undefined}
              />
            </Field>

            <div className="flex gap-2">
              <Button type="button" size="sm" disabled={connectMutation.isPending} onClick={submit}>
                {connectMutation.isPending ? <SpinnerIcon className="size-4 animate-spin" /> : null}
                {notConnected ? 'Connect' : 'Save'}
              </Button>
              {!notConnected ? (
                <Button type="button" size="sm" variant="outline" disabled={connectMutation.isPending} onClick={cancelEdit}>
                  Cancel
                </Button>
              ) : null}
            </div>
          </div>
        ) : query.data ? (
          <div className="space-y-3">
            <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1.5 text-sm">
              <dt className="text-muted-foreground">Repository</dt>
              <dd className="min-w-0 truncate font-mono">{query.data.repo_url}</dd>
              <dt className="text-muted-foreground">Branch</dt>
              <dd className="font-mono">{query.data.branch}</dd>
              <dt className="text-muted-foreground">Build pack</dt>
              <dd>{BUILD_PACKS.find((p) => p.value === query.data.build_type)?.label ?? query.data.build_type}</dd>
              <dt className="text-muted-foreground">Deploy token</dt>
              <dd>{query.data.has_token ? 'Configured' : 'Not set (public repo)'}</dd>
              {Object.keys(query.data.additional_services ?? {}).length > 0 ? (
                <>
                  <dt className="text-muted-foreground">Also deploys</dt>
                  <dd className="font-mono">
                    {Object.keys(query.data.additional_services ?? {}).join(', ')}
                  </dd>
                </>
              ) : null}
            </dl>

            <Field>
              <FieldLabel htmlFor="git-source-webhook-url-connected">Webhook URL</FieldLabel>
              <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
                <code id="git-source-webhook-url-connected" className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                  {webhookFullURL(query.data.webhook_url)}
                </code>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    if (query.data) {
                      copyText(webhookFullURL(query.data.webhook_url), setUrlCopied)
                    }
                  }}
                >
                  {urlCopied ? <CheckIcon /> : <CopyIcon />}
                  {urlCopied ? 'Copied' : 'Copy'}
                </Button>
              </div>
              <FieldDescription>
                The webhook secret was only ever shown once, at connect time.
                Disconnect and reconnect to rotate it.
              </FieldDescription>
            </Field>

            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => { startEdit(formFromResource(query.data)) }}
              >
                Edit
              </Button>
              <Button
                type="button"
                size="sm"
                variant="destructive"
                disabled={deleteGitSource.isPending}
                onClick={disconnect}
              >
                {deleteGitSource.isPending ? <SpinnerIcon className="size-4 animate-spin" /> : null}
                Disconnect
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
