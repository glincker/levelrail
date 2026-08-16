import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  GitBranchIcon,
  SpinnerIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldHint,
  FieldLabel,
} from '@/components/ui/field'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import { BrandIcon, type BrandIconName } from '@/components/BrandIcon'
import {
  useDeleteGitSource,
  useGitSource,
  useSetGitSource,
} from '../queries/gitSources'
import { ApiError } from '../lib/apiError'
import type { AppDetail } from '../types/appDetail'
import type { GitSourceBuildType, GitSourceResource } from '../types/gitSource'

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
// so this component's only chance to show it is the PUT response the
// instant a repo is first connected.
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

interface FormState {
  repoUrl: string
  branch: string
  buildType: GitSourceBuildType
  buildPath: string
  token: string
}

// Auto-detect is the default and recommended choice: most repos need no
// build configuration at all beyond picking their language, so a blank
// connect form should not default to the manual Dockerfile path.
function emptyForm(): FormState {
  return { repoUrl: '', branch: '', buildType: 'railpack', buildPath: '', token: '' }
}

function formFromResource(g: GitSourceResource): FormState {
  return {
    repoUrl: g.repo_url,
    branch: g.branch,
    buildType: g.build_type,
    buildPath: g.build_path ?? '',
    token: '',
  }
}

export function GitSourceCard({ app }: { app: AppDetail }) {
  const query = useGitSource(app.name)
  const setGitSource = useSetGitSource(app.name)
  const deleteGitSource = useDeleteGitSource(app.name)

  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<FormState>(emptyForm)
  // The one and only place the generated webhook secret is ever held:
  // set from setGitSource's own mutation response (only present on the
  // create path, gitSourceResource's own doc comment), cleared the
  // moment this card unmounts a connect/edit form. Never re-derived from
  // query.data, which never carries it.
  const [justConnected, setJustConnected] = useState<GitSourceResource | null>(null)
  const [secretCopied, setSecretCopied] = useState(false)
  const [urlCopied, setUrlCopied] = useState(false)

  const notConnected = query.error instanceof ApiError && query.error.status === 404
  const otherError = query.error && !notConnected ? query.error : null

  function startEdit(prefill: FormState) {
    setForm(prefill)
    setEditing(true)
    setJustConnected(null)
  }

  function cancelEdit() {
    setEditing(false)
    setForm(emptyForm())
  }

  function submit() {
    if (!form.repoUrl.trim()) {
      toast.add({ title: 'Repository URL is required.', type: 'error' })
      return
    }
    setGitSource.mutate(
      {
        repo_url: form.repoUrl.trim(),
        branch: form.branch.trim() || undefined,
        build_type: form.buildType,
        build_path: form.buildPath.trim() || undefined,
        token: form.token.trim() || undefined,
      },
      {
        onSuccess: (resource) => {
          setEditing(false)
          setForm(emptyForm())
          setSecretCopied(false)
          setUrlCopied(false)
          if (resource.webhook_secret) {
            setJustConnected(resource)
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
            <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
              <WarningIcon className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                This webhook secret will not be shown again. Paste both values into
                the repository&apos;s webhook settings now.
              </p>
            </div>

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

            <Button type="button" size="sm" onClick={() => { setJustConnected(null) }}>
              Done
            </Button>
          </div>
        ) : editing || notConnected ? (
          <div className="space-y-4">
            {!notConnected ? null : (
              <p className="text-sm text-muted-foreground">
                Connect a repository so a push to its target branch auto-deploys
                this app.
              </p>
            )}

            <Field>
              <FieldLabel htmlFor="git-source-repo-url">Repository URL</FieldLabel>
              <Input
                id="git-source-repo-url"
                className="font-mono"
                placeholder="https://github.com/you/app.git"
                autoComplete="off"
                spellCheck={false}
                value={form.repoUrl}
                onChange={(e) => { setForm({ ...form, repoUrl: e.target.value }) }}
                disabled={setGitSource.isPending}
              />
            </Field>

            <Field>
              <FieldLabel htmlFor="git-source-branch">Branch</FieldLabel>
              <Input
                id="git-source-branch"
                className="font-mono"
                placeholder="main"
                autoComplete="off"
                spellCheck={false}
                value={form.branch}
                onChange={(e) => { setForm({ ...form, branch: e.target.value }) }}
                disabled={setGitSource.isPending}
              />
            </Field>

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
                  <TabsTrigger value="railpack" disabled={setGitSource.isPending}>
                    Auto-detect
                  </TabsTrigger>
                  <TabsTrigger value="dockerfile" disabled={setGitSource.isPending}>
                    Dockerfile
                  </TabsTrigger>
                  <TabsTrigger value="static" disabled={setGitSource.isPending}>
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
                      disabled={setGitSource.isPending}
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
              <FieldLabel htmlFor="git-source-token">
                Deploy token (optional, for a private repo)
              </FieldLabel>
              <Input
                id="git-source-token"
                type="password"
                autoComplete="off"
                spellCheck={false}
                placeholder={notConnected ? 'Personal access token' : 'Leave blank to keep the current token'}
                value={form.token}
                onChange={(e) => { setForm({ ...form, token: e.target.value }) }}
                disabled={setGitSource.isPending}
              />
              <FieldDescription>
                Sent once, encrypted at rest. Never shown again after saving.
              </FieldDescription>
              <FieldHint
                href="https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens"
                linkText="How do I create one?"
              >
                Needs the <code className="text-xs">repo</code> scope (classic
                token) or Contents: Read-only permission (fine-grained token)
                so this control plane can clone the repository over HTTPS.
              </FieldHint>
            </Field>

            <div className="flex gap-2">
              <Button type="button" size="sm" disabled={setGitSource.isPending} onClick={submit}>
                {setGitSource.isPending ? <SpinnerIcon className="size-4 animate-spin" /> : null}
                {notConnected ? 'Connect' : 'Save'}
              </Button>
              {!notConnected ? (
                <Button type="button" size="sm" variant="outline" disabled={setGitSource.isPending} onClick={cancelEdit}>
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
