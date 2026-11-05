import { useEffect, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DialogFooter } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldError,
  FieldLabel,
  FieldSectionLabel,
} from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateApp } from '../queries/apps'
import { triggerBuild, type TriggerBuildInput } from '../queries/builds'
import { deployKeys } from '../queries/deploys'
import { deployAttemptKeys } from '../queries/deployAttempts'
import { gitSourceKeys } from '../queries/gitSources'
import {
  DEFAULT_VALUES,
  buildInputFrom,
  connectGitSourceFor,
  createAppFromGitSchema,
  repoSlugFrom,
  type FormInput,
  type FormOutput,
} from '../lib/createAppFromGit'
import { useFormDraft } from '../hooks/useFormDraft'
import { healthCheckFrom } from '../lib/healthCheckDefaults'
import { DraftRestoredNotice } from './DraftRestoredNotice'
import type { FrameworkPreset } from '../queries/frameworks'
import { FrameworkPresetPicker } from './FrameworkPresetPicker'
import { GitBuildSourceFields } from './GitBuildSourceFields'
import { GitDeployStatusAlerts } from './GitDeployStatusAlerts'
import {
  GitRepoSourcePicker,
  type GitRepoSourceValue,
} from './GitRepoSourcePicker'
import { CreateAppFromGitNetworkFields } from './CreateAppFromGitNetworkFields'
import { GitSourceWebhookBanner } from './GitSourceWebhookBanner'

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
  // The latest POST /api/v1/build/detect result for whatever repo/ref is
  // currently in the form (see GitBuildSourceFields' own onDetected doc
  // comment), sent along with the build trigger purely to be stored on
  // the resulting deploy attempt. undefined whenever nothing was
  // detected, detection hasn't run yet, or buildType is "image".
  const [detectedFramework, setDetectedFramework] = useState<
    string | undefined
  >(undefined)
  const [detectedFrameworkId, setDetectedFrameworkId] = useState<
    string | undefined
  >(undefined)
  // See this component's own doc comment for why this wraps
  // queries/builds.ts's triggerBuild() directly instead of using that
  // module's useTriggerBuild(appName) hook.
  const buildMutation = useMutation({
    meta: { errorToast: false },
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
    meta: { errorToast: false },
    mutationFn: ({ name, values }: { name: string; values: FormOutput }) =>
      connectGitSourceFor(name, values, source),
    onSuccess: (result, variables) => {
      queryClient.setQueryData(
        gitSourceKeys.detail(variables.name),
        result.resource,
      )
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
      setDetectedFramework(undefined)
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createApp/connectSourceMutation/buildMutation identity churn on
    // every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  // No secret field lives in this form's own values: the manual "deploy
  // token" GitRepoSourcePicker collects for a private repo stays in that
  // component's own local state (see `source` above), never flows into
  // this form's RHF values, so nothing needs excluding from the draft.
  const { restoredFromDraft, discardDraft, dismissDraftNotice, clearDraft } =
    useFormDraft({
      storageKey: 'app-create-app-git-draft',
      open,
      watch,
      reset,
      defaultValues: DEFAULT_VALUES,
    })
  const healthCheckEnabled = watch('healthCheckEnabled')
  const applyFrameworkPreset = (preset: FrameworkPreset) => {
    const opts = { shouldValidate: true, shouldDirty: true }
    if (preset.build_type !== 'dockerfile') {
      setValue('buildType', preset.build_type, opts)
    }
    if (preset.port > 0) setValue('port', String(preset.port), opts)
    setValue('healthCheckEnabled', true, opts)
    setValue('healthCheckPath', preset.health_path, opts)
  }

  function runBuild(name: string, values: FormOutput) {
    buildMutation.mutate(
      { name, input: buildInputFrom(values, detectedFramework) },
      {
        onSuccess: (result) => {
          clearDraft()
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
        {
          onSettled: () => {
            runBuild(name, values)
          },
        },
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
        health: healthCheckFrom(
          values.healthCheckEnabled,
          values.healthCheckPath,
        ),
      },
      {
        onSuccess: (created) => {
          proceedAfterCreate(created.name, values)
        },
      },
    )
  })

  const busy =
    createApp.isPending ||
    connectSourceMutation.isPending ||
    buildMutation.isPending
  // Locks name/port/domain forever once the app record exists: none of
  // the three are resent on a retry (see buildInputFrom and onSubmit
  // above), so editing them after step 1 succeeded would silently have
  // no effect. Domain already tells the user it can be changed later
  // from the app's Domains tab.
  const locked = createdName !== null
  const autoNameRef = useRef('')
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
      {restoredFromDraft ? (
        <DraftRestoredNotice
          onDiscard={discardDraft}
          onDismiss={dismissDraftNotice}
        />
      ) : null}

      <div className="space-y-4">
        <FieldSectionLabel>Source</FieldSectionLabel>
        {watch('buildType') !== 'image' ? (
          <GitRepoSourcePicker
            disabled={buildFieldsLocked}
            onSelect={(value) => {
              setSource(value)
              setValue('repoUrl', value.repoUrl, { shouldValidate: true })
              setValue('ref', value.branch, { shouldValidate: true })
              const current = getValues('name').trim()
              const slug = repoSlugFrom(value.repoUrl)
              if (slug && (!current || current === autoNameRef.current)) {
                setValue('name', slug, { shouldValidate: true })
                autoNameRef.current = slug
              }
            }}
          />
        ) : null}

        <Field>
          <FieldLabel htmlFor="git-app-name">Name</FieldLabel>
          <Input
            id="git-app-name"
            placeholder="e.g. marketing-site"
            disabled={locked}
            {...register('name')}
          />
          <FieldError errors={[formState.errors.name]} />
        </Field>
      </div>

      <div className="space-y-4">
        <FieldSectionLabel>Build</FieldSectionLabel>
        <GitBuildSourceFields
          control={control}
          register={register}
          formState={formState}
          getValues={getValues}
          setValue={setValue}
          watch={watch}
          disabled={buildFieldsLocked}
          pickedFromPicker={source !== null}
          onDetected={(result) => {
            setDetectedFramework(
              result?.detected ? result.frameworkName : undefined,
            )
            setDetectedFrameworkId(
              result?.detected ? result.frameworkId : undefined,
            )
          }}
        />
        <FrameworkPresetPicker
          detectedId={detectedFrameworkId}
          port={
            typeof watch('port') === 'string' ||
            typeof watch('port') === 'number'
              ? String(watch('port'))
              : ''
          }
          healthPath={watch('healthCheckPath')}
          disabled={buildFieldsLocked}
          onApply={applyFrameworkPreset}
        />
      </div>

      <CreateAppFromGitNetworkFields
        register={register}
        formState={formState}
        setValue={setValue}
        healthCheckEnabled={healthCheckEnabled}
        locked={locked}
      />

      <GitDeployStatusAlerts
        connectingSource={connectSourceMutation.isPending}
        triggeringBuild={buildMutation.isPending}
        createAppError={createApp.isError ? createApp.error.message : undefined}
        connectSourceError={
          connectSourceMutation.isError
            ? connectSourceMutation.error.message
            : undefined
        }
        buildError={
          buildMutation.isError ? buildMutation.error.message : undefined
        }
      />

      {connectSourceMutation.data &&
      !connectSourceMutation.data.autoRegistered &&
      connectSourceMutation.data.resource.webhook_secret ? (
        <GitSourceWebhookBanner
          webhookUrl={`${window.location.origin}${connectSourceMutation.data.resource.webhook_url}`}
          webhookSecret={connectSourceMutation.data.resource.webhook_secret}
          webhookError={connectSourceMutation.data.webhookError}
        />
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
