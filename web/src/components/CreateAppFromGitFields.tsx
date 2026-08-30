import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import {
  CheckIcon,
  CopyIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldHint,
  FieldLabel,
} from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateApp } from '../queries/apps'
import { triggerBuild, type TriggerBuildInput } from '../queries/builds'
import { deployKeys } from '../queries/deploys'
import { deployAttemptKeys } from '../queries/deployAttempts'
import { gitSourceKeys, setGitSource } from '../queries/gitSources'
import { connectGitLabProjectAsSource } from '../queries/gitlabApp'
import { connectBitbucketRepoAsSource } from '../queries/bitbucketApp'
import type { GitSourceBuildType, GitSourceResource } from '../types/gitSource'
import { GitBuildSourceFields } from './GitBuildSourceFields'
import {
  GitRepoSourcePicker,
  type GitRepoSourceValue,
} from './GitRepoSourcePicker'

// Build packs this form offers, matching GitBuildSourceFields' own tab
// picker for what POST /api/v1/apps/{name}/builds actually supports:
// dockerfile, railpack, static, image. See that component's doc comment
// for why Nixpacks and compose are never offered.
const BUILD_TYPES = ['dockerfile', 'railpack', 'static', 'image'] as const

// Same pattern DomainEditor.tsx validates a domain row against; kept as
// a local copy rather than exported/imported from there since this form
// only ever needs one domain, not that component's add/remove list.
const DOMAIN_PATTERN =
  /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/i

// Matches validateAppResource (internal/api/apps.go) for the fields
// this form actually collects on the create-app side, plus the git
// source fields POST /api/v1/apps/{name}/builds
// (internal/api/builds.go's handleTriggerBuild) requires. Mirrors
// DeployTriggerForm.tsx's own buildSchema for repoUrl/ref/imageRepo/
// dockerfilePath, since this is the same request shape, just fired
// once right after creating the app record instead of against an
// app that already exists. buildType is new: this form used to fire
// every build as an implicit build.type: dockerfile, matching what
// handleTriggerBuild used to reject everything else as.
//
// repoUrl/ref/image are only conditionally required (see the
// superRefine below): build.type: image needs image but not repoUrl/ref
// (handleTriggerBuild clones nothing for it), every other build type is
// the reverse.
const createAppFromGitSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    port: z.coerce
      .number({ error: 'Port is required' })
      .int('Port must be a whole number')
      .positive('Port must be a positive integer'),
    repoUrl: z.string().trim(),
    ref: z.string().trim(),
    imageRepo: z.string().trim(),
    buildType: z.enum(BUILD_TYPES),
    dockerfilePath: z.string().trim(),
    baseDirectory: z.string().trim(),
    // Dockerfile build-time ARGs, buildType: 'dockerfile' only. A blank
    // key is dropped in buildInputFrom below, not rejected here.
    buildArgs: z.array(z.object({ key: z.string(), value: z.string() })),
    image: z.string().trim(),
    domain: z
      .string()
      .trim()
      .refine(
        (v) => v === '' || DOMAIN_PATTERN.test(v),
        'Enter a valid domain, e.g. app.example.com',
      ),
  })
  .superRefine((values, ctx) => {
    if (values.buildType === 'image') {
      if (!values.image.trim()) {
        ctx.addIssue({
          code: 'custom',
          message: 'Image reference is required',
          path: ['image'],
        })
      }
      return
    }
    if (!values.repoUrl.trim()) {
      ctx.addIssue({
        code: 'custom',
        message: 'Repository URL is required',
        path: ['repoUrl'],
      })
    }
    if (!values.ref.trim()) {
      ctx.addIssue({
        code: 'custom',
        message: 'Branch, tag, or commit is required',
        path: ['ref'],
      })
    }
    if (values.buildType === 'dockerfile') {
      const seen = new Set<string>()
      values.buildArgs.forEach((arg, index) => {
        const key = arg.key.trim()
        if (!key) return
        if (seen.has(key)) {
          ctx.addIssue({
            code: 'custom',
            message: 'Duplicate key',
            path: ['buildArgs', index, 'key'],
          })
        }
        seen.add(key)
      })
    }
  })

export type FormInput = z.input<typeof createAppFromGitSchema>
export type FormOutput = z.output<typeof createAppFromGitSchema>

const DEFAULT_VALUES: FormInput = {
  name: '',
  port: '',
  repoUrl: '',
  ref: '',
  imageRepo: '',
  buildType: 'railpack',
  dockerfilePath: '',
  baseDirectory: '',
  buildArgs: [],
  image: '',
  domain: '',
}

// build.path is only ever sent for build.type: dockerfile: railpack has
// no path concept at all (handleTriggerBuild rejects one being set, see
// internal/api/builds.go's triggerBuildBuildInput.Path doc comment), and
// static's own build.path (the output subdirectory to serve) isn't
// exposed by this form yet, see GitBuildSourceFields' own doc comment
// for why. image_repo is only meaningful for a container-backed build
// (deployStatic never reads req.ImageRepo), so it's omitted for static
// too, matching GitBuildSourceFields not even rendering that field in
// that case. build.type: image sends only image: repoUrl/ref/imageRepo
// are all meaningless when nothing gets cloned or built. baseDirectory
// is sent for every non-image build type: the backend accepts it for
// dockerfile, railpack, and static alike. build.args is dockerfile-only,
// same restriction handleTriggerBuild enforces server-side.
function buildInputFrom(values: FormOutput): TriggerBuildInput {
  if (values.buildType === 'image') {
    return {
      repoUrl: '',
      ref: '',
      buildType: 'image',
      image: values.image.trim(),
    }
  }
  return {
    repoUrl: values.repoUrl.trim(),
    ref: values.ref.trim(),
    imageRepo:
      values.buildType === 'static'
        ? undefined
        : values.imageRepo.trim() || undefined,
    buildType: values.buildType,
    buildPath:
      values.buildType === 'dockerfile'
        ? values.dockerfilePath.trim() || undefined
        : undefined,
    baseDirectory: values.baseDirectory.trim() || undefined,
    buildArgs:
      values.buildType === 'dockerfile'
        ? buildArgsRecord(values.buildArgs)
        : undefined,
  }
}

// buildArgsRecord drops any row with a blank key, the same leniency
// this form's superRefine above already applies when checking for
// duplicates: a trailing blank row left over from "Add build arg" isn't
// a validation error, it's just not sent.
function buildArgsRecord(
  args: FormOutput['buildArgs'],
): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const { key, value } of args) {
    const trimmedKey = key.trim()
    if (trimmedKey) out[trimmedKey] = value
  }
  return Object.keys(out).length > 0 ? out : undefined
}

// repoSlugFrom turns a clone URL into a reasonable app-name suggestion,
// e.g. "https://github.com/acme/marketing-site.git" -> "marketing-site".
// Only ever used to prefill the Name field when it's still blank (see
// GitRepoSourcePicker's onSelect handler below), never to overwrite
// something the operator already typed.
function repoSlugFrom(repoUrl: string): string {
  const cleaned = repoUrl.trim().replace(/\.git$/i, '').replace(/\/+$/, '')
  const segments = cleaned.split(/[/:]/).filter(Boolean)
  const last = segments[segments.length - 1] ?? ''
  return last
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// gitSourceBuildFields mirrors this form's own build-pack choice into the
// git_source row's build_type/build_path, so a future webhook-triggered
// build (once the connect call below succeeds) uses the same build
// config as the very first one, not the git_source defaults. Only called
// once buildType !== 'image' is already known true.
function gitSourceBuildFields(
  values: FormOutput,
): { buildType: GitSourceBuildType; buildPath?: string } {
  const buildType = values.buildType as GitSourceBuildType
  return {
    buildType,
    buildPath:
      buildType === 'dockerfile' ? values.dockerfilePath.trim() || undefined : undefined,
  }
}

// connectGitSourceFor is the fix for both bugs docs-local/research/git-
// provider-connect-ux-unification-proposal.md documents: every provider,
// including GitHub, now gets an actual git_source row connected between
// app creation and the first build, not just GitLab/Bitbucket.
//
// `source` is trusted only when its repoUrl/branch still match what's
// actually in the form: GitBuildSourceFields' own Repository URL/Branch
// inputs stay directly editable after a provider pick prefills them, so
// a hand-edit after picking must not silently connect the wrong project.
// A mismatch (or no pick at all, e.g. the operator typed straight into
// GitBuildSourceFields) falls back to the generic endpoint, exactly the
// path GitSourceCard.tsx's own manual connect form already uses.
//
// GitLab and Bitbucket both have a real "use as source" endpoint that
// registers a push webhook automatically (queries/gitlabApp.ts,
// queries/bitbucketApp.ts). GitHub does not yet (see
// GitRepoSourcePicker.tsx's own doc comment: no repo-hook registration
// call exists in internal/githubapp/client.go), so a GitHub pick degrades
// to the same generic endpoint manual mode uses, which stores the
// git_source row and returns a webhook URL/secret to add by hand. That
// degradation, and closing GitHub's own auto-registration gap, is PR 2's
// job per the proposal, not this one's.
async function connectGitSourceFor(
  name: string,
  values: FormOutput,
  source: GitRepoSourceValue | null,
): Promise<{ resource: GitSourceResource; autoRegistered: boolean }> {
  const repoUrl = values.repoUrl.trim()
  const branch = values.ref.trim()
  const { buildType, buildPath } = gitSourceBuildFields(values)
  const effective =
    source && source.repoUrl === repoUrl && source.branch === branch ? source : null

  if (effective?.providerRef?.kind === 'gitlab') {
    const resource = await connectGitLabProjectAsSource(effective.providerRef.projectId, {
      app_name: name,
      branch,
      build_type: buildType,
      build_path: buildPath,
    })
    return { resource, autoRegistered: true }
  }
  if (effective?.providerRef?.kind === 'bitbucket') {
    const resource = await connectBitbucketRepoAsSource(
      effective.providerRef.workspace,
      effective.providerRef.repoSlug,
      {
        app_name: name,
        branch,
        build_type: buildType,
        build_path: buildPath,
      },
    )
    return { resource, autoRegistered: true }
  }
  const resource = await setGitSource(name, {
    repo_url: repoUrl,
    branch,
    build_type: buildType,
    build_path: buildPath,
    token: effective?.provider === 'manual' ? effective.token : undefined,
  })
  return { resource, autoRegistered: false }
}

// Placeholder image tag POST /api/v1/apps is sent on step 1 of this
// form's two-request sequence (create the app record, then trigger a
// build). See CreateAppFromGitFields's own doc comment for the full
// "why a placeholder" reasoning; exported so it stays discoverable
// (e.g. from a future test) rather than a silent magic string.
export const PENDING_BUILD_TAG = 'pending-build'

// The "Dockerfile from git" step 2 path CreateResourceWizard adds: no
// existing dialog covers this (CreateAppDialog is the "already have a
// built image" path, DeployTriggerForm's own BuildFromSourceForm only
// ever runs against an app that already exists). Two backend calls,
// run in sequence:
//
//   1. POST /api/v1/apps (handleCreateApp) to create the app record.
//      validateAppResource requires a non-empty image and a positive
//      port, but there is no built image yet at this point in the
//      flow, only a git source, so this step sends `${name}:pending-build`
//      as a legible, obviously-not-final placeholder rather than
//      leaving the field looking like a real, working image tag if the
//      next step never completes.
//   2. POST /api/v1/apps/{name}/builds (handleTriggerBuild), which
//      triggers the build and returns immediately (the build itself runs
//      asynchronously on the server, see queries/builds.ts's own doc
//      comment). Once it finishes, it overwrites the app's desired image
//      via internal/deploy.Pipeline.deployDockerfile's own
//      store.SaveDesiredService call (internal/deploy/deploy.go): the
//      placeholder from step 1 stays visible until then, or is left in
//      place if the build fails.
//
// No prior precedent for this exact two-step sequencing exists anywhere
// in the codebase (grepped internal/api, internal/deploy,
// internal/webhook: every existing build/deploy path always operates on
// an app that was already created some other way). This is a
// deliberate, new design for this task, not a rediscovery of an
// existing pattern.
//
// A brief window exists between step 1 and step 2 where the reconciler
// could observe the placeholder image as this app's desired state and
// attempt (and fail) to pull it before the build finishes and
// overwrites it. This is accepted rather than engineered around:
// reconcilers in this codebase are level-triggered and idempotent by
// design, so a transient failed-pull condition during
// that window self-heals on the next reconcile once the real image
// lands, the same way any other transient desired/observed mismatch
// already does.
//
// The build mutation here is a local useMutation over queries/builds.ts's
// exported triggerBuild() fetcher, not that module's own useTriggerBuild
// hook: useTriggerBuild binds the target app name at hook-creation time
// (useTriggerBuild(appName)), but this form doesn't know the app's name
// until createApp's own onSuccess fires with the server's response, one
// render after the state that name would live in updates. Calling
// triggerBuild.mutate synchronously inside createApp's onSuccess would
// still be closed over the previous render's hook instance, bound to
// the stale (pre-creation) name. Parameterizing name per mutate() call
// instead sidesteps that entirely; the onSuccess cache-writing logic
// below is copied from useTriggerBuild verbatim so behavior stays
// identical either way.
//
// Node placement (the optional Node select CreateAppFields/
// CreateDatabaseFields both offer) is deliberately not included here:
// the task's own "minimal config" framing plus the fact that
// DeployTriggerForm's own BuildFromSourceForm, this form's closest
// sibling, has no node field either, since it always operates on an
// app whose placement was already decided. Can be added later as a
// fast-follow if wanted.
export function CreateAppFromGitFields({
  open,
  onCreated,
}: {
  /** The owning dialog's own open state, used only to reset this
   *  form's local state on close. See CreateAppFields's identical prop
   *  for the full reasoning. */
  open: boolean
  /** Called once the build has been triggered (not once it has
   *  finished, see queries/builds.ts's own doc comment) and navigation
   *  to the app's detail page has been kicked off. Not called if
   *  triggering the build itself fails: the app record already exists
   *  by then (step 1 succeeded), so the dialog stays open and the same
   *  submit button retries rather than dead-ending. */
  onCreated: () => void
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const createApp = useCreateApp()
  // Whether step 1 (POST /api/v1/apps) has already succeeded for this
  // open session, so a resubmission after a build failure retries only
  // the build: the name now exists, a second createApp call would
  // 409 on it. Derived from the mutation's own last response rather
  // than tracked in separate state: createApp.reset() (called on close
  // below) already clears .data back to undefined, so there's nothing
  // extra to keep in sync by hand.
  const createdName = createApp.data?.name ?? null
  // The last repo/branch/provider picked from GitRepoSourcePicker, kept
  // separate from the zod-validated form fields (repoUrl/ref) since it
  // carries provider metadata (which connect endpoint to call) that has
  // no wire representation of its own. See connectGitSourceFor's own doc
  // comment for how a stale value here (the form fields hand-edited after
  // a pick) gets detected and ignored.
  const [source, setSource] = useState<GitRepoSourceValue | null>(null)
  // Copy-button state for the "add this webhook by hand" banner below,
  // shown only for the generic (non-auto-registering) connect path. Reset
  // is unnecessary beyond the dialog's own close-resets-everything effect:
  // a fresh connect always renders a fresh banner with these starting
  // false again anyway.
  const [webhookUrlCopied, setWebhookUrlCopied] = useState(false)
  const [webhookSecretCopied, setWebhookSecretCopied] = useState(false)
  // See this component's own doc comment for why this wraps
  // queries/builds.ts's triggerBuild() directly instead of using that
  // module's useTriggerBuild(appName) hook.
  const buildMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: TriggerBuildInput }) =>
      triggerBuild(name, input),
    onSuccess: (_result, variables) => {
      // The build hasn't finished yet at this point (see
      // queries/builds.ts's own doc comment), so there is no updated app
      // or static site to write into the cache: invalidate instead, the
      // same split useTriggerBuild's own onSuccess makes.
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(variables.name),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(variables.name),
      })
    },
  })
  // Connects the git source between app creation and the first build, for
  // every provider (see connectGitSourceFor's own doc comment for the two
  // bugs this fixes). Not gated on `enabled`/`retry`: proceedAfterCreate
  // below only calls mutate() when there's actually a repo to connect, and
  // skips calling it again once it has already succeeded once this
  // session (isSuccess), so a build-only retry never re-registers a
  // second webhook on GitLab/Bitbucket.
  const connectSourceMutation = useMutation({
    mutationFn: ({ name, values }: { name: string; values: FormOutput }) =>
      connectGitSourceFor(name, values, source),
    onSuccess: (result, variables) => {
      queryClient.setQueryData(gitSourceKeys.detail(variables.name), result.resource)
    },
  })
  const {
    register,
    handleSubmit,
    formState,
    reset,
    control,
    watch,
    getValues,
    setValue,
  } = useForm<FormInput, unknown, FormOutput>({
    resolver: zodResolver(createAppFromGitSchema),
    defaultValues: DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!open) {
      reset(DEFAULT_VALUES)
      createApp.reset()
      connectSourceMutation.reset()
      buildMutation.reset()
      setSource(null)
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createApp/connectSourceMutation/buildMutation identity churn on
    // every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function runBuild(name: string, values: FormOutput) {
    buildMutation.mutate(
      { name, input: buildInputFrom(values) },
      {
        onSuccess: (result) => {
          onCreated()
          toast.add({
            title: `App "${name}" created, build triggered.`,
            description: 'Watching the build log now.',
            type: 'success',
          })
          // result.id is the deploy attempt id (queries/builds.ts's own
          // doc comment: empty only if attempt recording itself failed,
          // which never blocks the build). Land on its live log view
          // instead of the plain app page, the same "watch it happen"
          // landing DeployAttemptsList's own Logs link gives an
          // already-existing app's rebuild.
          if (result.id) {
            void navigate({
              to: '/apps/$name/deploys/$deployId/logs',
              params: { name, deployId: result.id },
            })
          } else {
            void navigate({ to: '/apps/$name', params: { name } })
          }
        },
      },
    )
  }

  // proceedAfterCreate runs the second and third of the three steps every
  // provider now gets (connectGitSourceFor's own doc comment): connect
  // the git source, then trigger the build. A failed connect does not
  // block the build (onSettled, not onSuccess): the operator still gets a
  // running app either way, just without auto-deploy-on-push until they
  // reconnect it from the app's Overview page, the same degraded-but-
  // working outcome GitSourceCard.tsx's own connect failure already
  // leaves an existing app in.
  function proceedAfterCreate(name: string, values: FormOutput) {
    const needsSource =
      values.buildType !== 'image' &&
      values.repoUrl.trim() !== '' &&
      values.ref.trim() !== ''
    if (needsSource && !connectSourceMutation.isSuccess) {
      connectSourceMutation.mutate(
        { name, values },
        { onSettled: () => { runBuild(name, values) } },
      )
      return
    }
    runBuild(name, values)
  }

  const onSubmit = handleSubmit((values) => {
    if (createdName) {
      // Retry after a previous build failure: the app record already
      // exists, so this resubmission only retries the build (and, if it
      // didn't already succeed, the git source connect), using whatever
      // values are currently in the form (e.g. a corrected ref).
      proceedAfterCreate(createdName, values)
      return
    }
    createApp.mutate(
      {
        name: values.name.trim(),
        image: `${values.name.trim()}:${PENDING_BUILD_TAG}`,
        port: values.port,
        domains: values.domain.trim() ? [values.domain.trim()] : undefined,
      },
      {
        onSuccess: (created) => {
          proceedAfterCreate(created.name, values)
        },
      },
    )
  })

  const busy =
    createApp.isPending || connectSourceMutation.isPending || buildMutation.isPending
  // Locks name/port/domain forever once the app record exists: none of
  // the three are resent on a retry (see buildInputFrom and onSubmit
  // above), so editing them after step 1 succeeded would silently have
  // no effect. Domain already tells the user it can be changed later
  // from the app's Domains tab.
  const locked = createdName !== null
  // The git/build fields ARE resent on every retry, so a failed build
  // trigger must not leave them stuck disabled: that's the only way a
  // user can act on the "fix the fields above and submit again" error
  // below. Re-lock them while a request is actually in flight.
  const buildFieldsLocked = busy || (locked && !buildMutation.isError)

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      {watch('buildType') !== 'image' ? (
        <GitRepoSourcePicker
          disabled={buildFieldsLocked}
          onSelect={(value) => {
            setSource(value)
            setValue('repoUrl', value.repoUrl, { shouldValidate: true })
            setValue('ref', value.branch, { shouldValidate: true })
            if (!getValues('name').trim()) {
              const slug = repoSlugFrom(value.repoUrl)
              if (slug) {
                setValue('name', slug, { shouldValidate: true })
              }
            }
          }}
        />
      ) : null}

      <div className="flex flex-col gap-4 sm:flex-row">
        <Field className="flex-1">
          <FieldLabel htmlFor="git-app-name">Name</FieldLabel>
          <Input
            id="git-app-name"
            placeholder="e.g. marketing-site"
            disabled={locked}
            {...register('name')}
          />
          <FieldError errors={[formState.errors.name]} />
        </Field>

        <Field className="flex-1">
          <FieldLabel htmlFor="git-app-port">Port</FieldLabel>
          <Input
            id="git-app-port"
            type="number"
            step="1"
            min="1"
            placeholder="e.g. 3000"
            disabled={locked}
            {...register('port')}
          />
          <FieldHint>
            The port your app listens on inside its container, e.g. 3000 for a
            typical Next.js app.
          </FieldHint>
          <FieldError errors={[formState.errors.port]} />
        </Field>
      </div>

      <GitBuildSourceFields
        control={control}
        register={register}
        formState={formState}
        getValues={getValues}
        setValue={setValue}
        watch={watch}
        disabled={buildFieldsLocked}
      />

      <Field>
        <FieldLabel htmlFor="git-app-domain">Domain (optional)</FieldLabel>
        <Input
          id="git-app-domain"
          className="font-mono"
          placeholder="app.example.com"
          autoComplete="off"
          spellCheck={false}
          disabled={locked}
          {...register('domain')}
        />
        <FieldError errors={[formState.errors.domain]} />
        <FieldDescription>
          Routed once its DNS record points here. A TLS certificate is issued
          automatically, or add this later from the app&apos;s Domains tab.
        </FieldDescription>
      </Field>

      {connectSourceMutation.isPending ? (
        <Alert>
          <AlertDescription>Connecting the git source...</AlertDescription>
        </Alert>
      ) : null}

      {buildMutation.isPending ? (
        <Alert>
          <AlertDescription>Triggering the build...</AlertDescription>
        </Alert>
      ) : null}

      {createApp.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{createApp.error.message}</AlertDescription>
        </Alert>
      ) : null}

      {connectSourceMutation.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            App created, but connecting the git source failed:{' '}
            {connectSourceMutation.error.message}. The build below still runs;
            connect a git source afterward from the app&apos;s Overview page
            for automatic deploys on push.
          </AlertDescription>
        </Alert>
      ) : null}

      {connectSourceMutation.data && !connectSourceMutation.data.autoRegistered
      && connectSourceMutation.data.resource.webhook_secret ? (
        <Alert>
          <AlertDescription className="space-y-2">
            <p>
              Git source connected. This provider doesn&apos;t register a
              webhook automatically yet, add these to the repository&apos;s
              settings for pushes to auto-deploy:
            </p>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {window.location.origin}
                {connectSourceMutation.data.resource.webhook_url}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  void navigator.clipboard
                    .writeText(
                      `${window.location.origin}${connectSourceMutation.data?.resource.webhook_url ?? ''}`,
                    )
                    .then(() => { setWebhookUrlCopied(true) })
                }}
              >
                {webhookUrlCopied ? <CheckIcon /> : <CopyIcon />}
                {webhookUrlCopied ? 'Copied' : 'Copy'}
              </Button>
            </div>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {connectSourceMutation.data.resource.webhook_secret}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  const secret = connectSourceMutation.data?.resource.webhook_secret
                  if (secret) {
                    void navigator.clipboard.writeText(secret).then(() => {
                      setWebhookSecretCopied(true)
                    })
                  }
                }}
              >
                {webhookSecretCopied ? <CheckIcon /> : <CopyIcon />}
                {webhookSecretCopied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : null}

      {buildMutation.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            App created, but triggering the build failed:{' '}
            {buildMutation.error.message}. Fix the fields above and submit again
            to retry.
          </AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={busy}>
          {createApp.isPending
            ? 'Creating...'
            : connectSourceMutation.isPending
              ? 'Connecting...'
              : buildMutation.isPending
                ? 'Building...'
                : locked
                  ? 'Retry build'
                  : 'Build and deploy'}
        </Button>
      </DialogFooter>
    </form>
  )
}
