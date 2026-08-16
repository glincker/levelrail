import { useEffect } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { appKeys, useCreateApp } from '../queries/apps'
import { triggerBuild, type TriggerBuildInput } from '../queries/builds'
import { deployKeys } from '../queries/deploys'
import { staticSitesKeys } from '../queries/staticSites'
import { GitBuildSourceFields } from './GitBuildSourceFields'
import { GitHubAppRepoPicker } from './GitHubAppRepoPicker'

// Build packs this form offers, matching GitBuildSourceFields' own
// BUILD_PACKS list of what POST /api/v1/apps/{name}/builds actually
// supports: dockerfile, railpack, static. See that component's doc
// comment for why Nixpacks and compose are never offered.
const BUILD_TYPES = ['dockerfile', 'railpack', 'static'] as const

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
const createAppFromGitSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  port: z.coerce
    .number({ error: 'Port is required' })
    .int('Port must be a whole number')
    .positive('Port must be a positive integer'),
  repoUrl: z.string().trim().min(1, 'Repository URL is required'),
  ref: z.string().trim().min(1, 'Branch, tag, or commit is required'),
  imageRepo: z.string().trim(),
  buildType: z.enum(BUILD_TYPES),
  dockerfilePath: z.string().trim(),
})

export type FormInput = z.input<typeof createAppFromGitSchema>
export type FormOutput = z.output<typeof createAppFromGitSchema>

const DEFAULT_VALUES: FormInput = {
  name: '',
  port: '',
  repoUrl: '',
  ref: '',
  imageRepo: '',
  buildType: 'dockerfile',
  dockerfilePath: '',
}

// build.path is only ever sent for build.type: dockerfile: railpack has
// no path concept at all (handleTriggerBuild rejects one being set, see
// internal/api/builds.go's triggerBuildBuildInput.Path doc comment), and
// static's own build.path (the output subdirectory to serve) isn't
// exposed by this form yet, see GitBuildSourceFields' own doc comment
// for why. image_repo is only meaningful for a container-backed build
// (deployStatic never reads req.ImageRepo), so it's omitted for static
// too, matching GitBuildSourceFields not even rendering that field in
// that case.
function buildInputFrom(values: FormOutput): TriggerBuildInput {
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
  }
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
//      builds the image and, on success, overwrites the app's desired
//      image via internal/deploy.Pipeline.deployDockerfile's own
//      store.SaveDesiredService call (internal/deploy/deploy.go): the
//      placeholder from step 1 is only ever visible for however long
//      the build takes, or left in place if the build fails.
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
  /** Called once the build has succeeded and navigation to the app's
   *  detail page has been kicked off. Not called on a build failure:
   *  the app record already exists by then (step 1 succeeded), so the
   *  dialog stays open and the same submit button retries the build
   *  rather than dead-ending. */
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
  // See this component's own doc comment for why this wraps
  // queries/builds.ts's triggerBuild() directly instead of using that
  // module's useTriggerBuild(appName) hook.
  const buildMutation = useMutation({
    mutationFn: ({ name, input }: { name: string; input: TriggerBuildInput }) =>
      triggerBuild(name, input),
    onSuccess: (result, variables) => {
      // A build.type: static result carries no app (see
      // TriggerBuildResult's own doc comment in queries/builds.ts): the
      // app detail cache is left untouched rather than overwritten with
      // undefined, and the static-sites list is invalidated instead,
      // the same split useTriggerBuild's own onSuccess makes.
      if (result.app) {
        queryClient.setQueryData(appKeys.detail(variables.name), result.app)
      }
      if (result.static_site) {
        void queryClient.invalidateQueries({ queryKey: staticSitesKeys.all })
      }
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(variables.name),
      })
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
      buildMutation.reset()
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createApp/buildMutation identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  function runBuild(name: string, values: FormOutput) {
    buildMutation.mutate(
      { name, input: buildInputFrom(values) },
      {
        onSuccess: () => {
          onCreated()
          toast.add({
            title: `App "${name}" created and built.`,
            type: 'success',
          })
          void navigate({ to: '/apps/$name', params: { name } })
        },
      },
    )
  }

  const onSubmit = handleSubmit((values) => {
    if (createdName) {
      // Retry after a previous build failure: the app record already
      // exists, so this resubmission only retries the build, using
      // whatever values are currently in the form (e.g. a corrected
      // ref).
      runBuild(createdName, values)
      return
    }
    createApp.mutate(
      {
        name: values.name.trim(),
        image: `${values.name.trim()}:${PENDING_BUILD_TAG}`,
        port: values.port,
      },
      {
        onSuccess: (created) => {
          runBuild(created.name, values)
        },
      },
    )
  })

  const busy = createApp.isPending || buildMutation.isPending
  const locked = createdName !== null

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
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
          <FieldError errors={[formState.errors.port]} />
        </Field>
      </div>

      <GitHubAppRepoPicker
        onSelect={(repoUrl, ref) => {
          setValue('repoUrl', repoUrl, { shouldValidate: true })
          setValue('ref', ref, { shouldValidate: true })
        }}
      />

      <GitBuildSourceFields
        control={control}
        register={register}
        formState={formState}
        getValues={getValues}
        setValue={setValue}
        watch={watch}
        disabled={locked}
      />

      {buildMutation.isPending ? (
        <Alert>
          <AlertDescription>
            Building from source. This can take a while for a real image, there
            is no live progress log yet.
          </AlertDescription>
        </Alert>
      ) : null}

      {createApp.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{createApp.error.message}</AlertDescription>
        </Alert>
      ) : null}

      {buildMutation.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            App created, but the build failed: {buildMutation.error.message}.
            Fix the fields above and submit again to retry the build.
          </AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={busy}>
          {createApp.isPending
            ? 'Creating...'
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
