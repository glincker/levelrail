import { NewAppPreflight } from './PreflightPanel'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Controller } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Field,
  FieldError,
  FieldHint,
  FieldLabel,
  FieldSectionLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { FormErrorSummary } from '@/components/ui/form-error-summary'
import { useCreateApp } from '../queries/apps'
import { useNodeListOptional } from '../queries/nodes'
import { useProjectListOptional } from '../queries/projects'
import { getProjectScope } from '../lib/projectScope'
import { useFormDraft } from '../hooks/useFormDraft'
import { useAppForm } from '../hooks/useAppForm'
import {
  HEALTH_CHECK_DEFAULT_PATH,
  healthCheckFrom,
} from '../lib/healthCheckDefaults'
import {
  buildCreateResourceSuccessHandler,
  resolveSubmittedNodeId,
  resolveSubmittedProjectId,
} from '../lib/createResourcePlacement'
import { CreateFormShell } from './CreateFormShell'
import {
  AUTO_NODE_VALUE,
  LOCAL_NODE_VALUE,
  NO_PROJECT_VALUE,
  NodeSelectField,
  ProjectSelectField,
} from './PlacementFields'
import { RegistryImagePicker } from './RegistryImagePicker'

// Mirrors validateAppResource (internal/api/apps.go) client-side for
// fast feedback: name and image non-empty, port a positive integer.
// This is a sanity check, not a substitute for the server's own
// validation, on submit the real request still goes out and the real
// response (including a 409 on a duplicate name) is what onSubmit
// below shows, this schema only stops the obviously-invalid case from
// making a round trip at all.
//
// port uses z.coerce.number() the same way CreateAlertRuleDialog's
// threshold/restartCountThreshold fields do, since it's bound to a raw
// <input type="number">: the field-value type register/defaultValues
// bind to (z.input, effectively unknown for a coerced field) differs
// from the post-validation submit type (z.output, a real number), so
// useForm's generic needs both spelled out below, same reasoning that
// file's own comment gives.
// Sentinel for "use the store's own default", the same reasoning
// LOCAL_NODE_VALUE documents for the node field just above: base-ui's
// Select can't use an empty string as a real item value, and leaving
// strategy unset entirely is exactly what should happen when an
// operator doesn't touch this field (validateAppResource,
// internal/api/apps.go, already treats an empty Strategy as "fall
// through to store.DefaultDeployStrategy").
const STRATEGY_DEFAULT_VALUE = '__default__'

// Suggests an app name from a picked `host/repository:tag` image
// reference, e.g. "ghcr.io/you/marketing-site:latest" -> "marketing-site".
// Mirrors CreateAppFromGitFields.tsx's repoSlugFrom: only ever used to
// prefill Name while it's still blank, never to overwrite a typed value.
function imageSlugFrom(imageRef: string): string {
  const lastSegment = imageRef.trim().split('/').filter(Boolean).pop() ?? ''
  const withoutTag = (lastSegment.split('@')[0] ?? '').replace(/:[^:]*$/, '')
  return withoutTag
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

const createAppSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    image: z.string().trim().min(1, 'Image is required'),
    port: z.coerce
      .number({ error: 'Port is required' })
      .int('Port must be a whole number')
      .positive('Port must be a positive integer'),
    // Optional target node id, empty string means the local node. Only
    // ever populated from the Select below, which only ever offers real
    // node ids or the local sentinel, so no further validation is needed
    // beyond what the Select already constrains it to.
    node: z.string().optional(),
    // Optional project id, same shape as node above.
    project: z.string().optional(),
    strategy: z.enum([
      'recreate',
      'blue-green',
      'rolling',
      STRATEGY_DEFAULT_VALUE,
    ]),
    // Left as a plain trimmed string rather than z.coerce.number: an empty
    // string here means "use store.DefaultReplicas", which a coerced
    // number field can't represent (Number('') is 0, a real, different
    // value), so validation into a real optional number happens in
    // onSubmit below instead, after this schema has confirmed the field is
    // either blank or a valid positive integer string.
    replicas: z
      .string()
      .trim()
      .refine(
        (v) => v === '' || (/^\d+$/.test(v) && Number(v) > 0),
        'Replicas must be a positive whole number, or left blank for the default',
      ),
    healthCheckEnabled: z.boolean(),
    healthCheckPath: z.string().trim(),
  })
  .superRefine((values, ctx) => {
    if (!values.healthCheckEnabled) return
    if (!values.healthCheckPath) {
      ctx.addIssue({
        code: 'custom',
        message: 'Health check path is required',
        path: ['healthCheckPath'],
      })
    } else if (!values.healthCheckPath.startsWith('/')) {
      ctx.addIssue({
        code: 'custom',
        message: 'Path must start with /',
        path: ['healthCheckPath'],
      })
    }
  })

type CreateAppFormInput = z.input<typeof createAppSchema>
type CreateAppFormOutput = z.output<typeof createAppSchema>

const BASE_VALUES: CreateAppFormInput = {
  name: '',
  image: '',
  port: '',
  node: AUTO_NODE_VALUE,
  project: NO_PROJECT_VALUE,
  strategy: STRATEGY_DEFAULT_VALUE,
  replicas: '',
  // Mirrors the CLI's interactive wizard default (apps_create_
  // interactive.go): a health check is on by default at the documented
  // /healthz convention, since most apps benefit from one and it's easy
  // to turn off for the ones that don't implement it.
  healthCheckEnabled: true,
  healthCheckPath: HEALTH_CHECK_DEFAULT_PATH,
}

// New apps start in the project picked in the scope picker.
function initialValues(): CreateAppFormInput {
  return { ...BASE_VALUES, project: getProjectScope() || NO_PROJECT_VALUE }
}

// The name/image/port(/node/health) field set and its submit logic,
// factored out of what used to be CreateAppDialog's own body so the
// exact same validation/mutation code backs both that standalone dialog
// and CreateResourceWizard's step 2 "Docker image" path: one schema, one
// useCreateApp call, not copy-pasted a second time for the wizard.
// domains/env/resources are real fields the backend accepts but stay out
// of scope here, a manually-created app via this form is the "already
// have a built image" path, distinct from and unaffected by the
// git-push spec path.
export function CreateAppFields({
  open,
  onCreated,
}: {
  /** The owning dialog's own open state. Used only to reset this
   *  form's local state (react-hook-form values plus the mutation's
   *  own error state) the moment the dialog closes: base-ui's Dialog
   *  keeps its popup content mounted through its own closing animation
   *  rather than unmounting immediately (see ui/dialog.tsx's
   *  data-closed:animate-out), so this component can't rely on
   *  unmount to clear stale field values between one open and the
   *  next, the same way the original CreateAppDialog's own
   *  handleOpenChange used to call reset() directly. */
  open: boolean
  /** Called once the app has been created and navigation to its detail
   *  page has been kicked off. The caller (CreateAppDialog, or the
   *  wizard's step 2) uses this purely to close its own dialog/step
   *  state; this component has no "open" concept of its own. */
  onCreated: () => void
}) {
  const { t } = useTranslation('newApp')
  const navigate = useNavigate()
  const createApp = useCreateApp()
  // Optional convenience only, see useNodeListOptional's own doc
  // comment: a failure or empty list here must never block app
  // creation, so the node field below is simply not rendered rather
  // than surfacing a loading or error state of its own.
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: a failure or empty list here must never block app
  // creation, the project field below is simply not rendered.
  const projectList = useProjectListOptional()
  const projects = projectList.data ?? []
  // Strategy/replicas/node/project are real fields but not ones a
  // first-time deploy needs to touch, so they start collapsed. Not reset
  // on close: the wizard's own step-2 branch unmounts this component
  // entirely when the dialog closes (see CreateResourceWizard.tsx), so a
  // fresh open already gets a fresh useState(false) here.
  const [showAdvanced, setShowAdvanced] = useState(false)
  // With several nodes the choice matters, so it is not hidden.
  const multiNode = nodes.length > 1
  const form = useAppForm<CreateAppFormInput, CreateAppFormOutput>(
    createAppSchema,
    { defaultValues: initialValues() },
  )
  const {
    control,
    register,
    handleSubmit,
    formState,
    reset,
    watch,
    setValue,
    getValues,
  } = form

  useEffect(() => {
    if (!open) {
      reset(initialValues())
      createApp.reset()
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createApp identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const { restoredFromDraft, discardDraft, dismissDraftNotice, clearDraft } =
    useFormDraft({
      storageKey: 'app-create-app-draft',
      open,
      watch,
      reset,
      defaultValues: initialValues(),
    })
  const healthCheckEnabled = watch('healthCheckEnabled')

  const onSubmit = handleSubmit((values) => {
    const nodeId = values.node ?? LOCAL_NODE_VALUE
    createApp.mutate(
      {
        name: values.name.trim(),
        image: values.image.trim(),
        port: values.port,
        // Both left undefined (omitted from the request body entirely,
        // rather than sent as "" / 0) when the operator didn't touch
        // them, so the server's own default-fallthrough applies exactly
        // as it does for a service saved with no strategy/replicas at
        // all.
        strategy:
          values.strategy === STRATEGY_DEFAULT_VALUE
            ? undefined
            : values.strategy,
        replicas: values.replicas === '' ? undefined : Number(values.replicas),
        health: healthCheckFrom(
          values.healthCheckEnabled,
          values.healthCheckPath,
        ),
        project_id: resolveSubmittedProjectId(values.project),
        // Only sent once the operator has actually opened the advanced
        // panel: leaving node_id undefined (dropped from the JSON body
        // entirely) whenever they never touched it lets the server
        // auto-place this app via simple spread scheduling, see
        // CreateAppRequest's own doc comment (queries/apps.ts). Opening
        // advanced and submitting "Local node" (the field's own default)
        // still counts as an explicit choice here, sent as '', so it's
        // never silently overridden by that auto-placement.
        node_id: resolveSubmittedNodeId(showAdvanced || multiNode, nodeId),
      },
      {
        onSuccess: buildCreateResourceSuccessHandler({
          resourceLabel: 'App',
          clearDraft,
          onCreated,
          onNavigate: (name) => {
            void navigate({ to: '/apps/$name', params: { name } })
          },
        }),
      },
    )
  })

  return (
    <CreateFormShell
      onSubmit={onSubmit}
      restoredFromDraft={restoredFromDraft}
      onDiscardDraft={discardDraft}
      onDismissDraftNotice={dismissDraftNotice}
    >
      <FormErrorSummary
        form={form}
        fields={{
          name: { label: t('fields.name'), id: 'app-name' },
          image: { label: t('fields.image'), id: 'app-image' },
          port: { label: t('fields.port'), id: 'app-port' },
          healthCheckPath: {
            label: t('fields.healthPath'),
            id: 'app-health-path',
          },
          replicas: { label: t('fields.replicas'), id: 'app-replicas' },
        }}
      />
      <div className="space-y-4">
        <FieldSectionLabel>{t('fields.basics')}</FieldSectionLabel>
        <Field>
          <FieldLabel htmlFor="app-name">{t('fields.name')}</FieldLabel>
          <Input
            id="app-name"
            placeholder={t('fields.namePlaceholder')}
            {...register('name')}
          />
          <FieldError errors={[formState.errors.name]} />
        </Field>

        <RegistryImagePicker
          onSelect={(imageRef) => {
            setValue('image', imageRef, {
              shouldValidate: true,
              shouldDirty: true,
            })
            if (!getValues('name').trim()) {
              const slug = imageSlugFrom(imageRef)
              if (slug) {
                setValue('name', slug, {
                  shouldValidate: true,
                  shouldDirty: true,
                })
              }
            }
          }}
        />

        <Field>
          <FieldLabel htmlFor="app-image">{t('fields.image')}</FieldLabel>
          <Input
            id="app-image"
            placeholder={t('fields.imagePlaceholder')}
            {...register('image')}
          />
          <FieldHint>{t('fields.imageHint')}</FieldHint>
          <FieldError errors={[formState.errors.image]} />
        </Field>
      </div>

      <div className="space-y-4">
        <FieldSectionLabel>{t('fields.networking')}</FieldSectionLabel>
        <Field>
          <FieldLabel htmlFor="app-port">{t('fields.port')}</FieldLabel>
          <Input
            id="app-port"
            type="number"
            step="1"
            min="1"
            placeholder={t('fields.portPlaceholder')}
            {...register('port')}
          />
          <FieldHint>{t('fields.portHint')}</FieldHint>
          <FieldError errors={[formState.errors.port]} />
        </Field>
      </div>

      <div className="space-y-4">
        <FieldSectionLabel>{t('fields.healthCheck')}</FieldSectionLabel>
        <Field orientation="horizontal">
          <Controller
            control={control}
            name="healthCheckEnabled"
            render={({ field }) => (
              <Switch
                id="app-health-enabled"
                checked={field.value}
                onCheckedChange={field.onChange}
              />
            )}
          />
          <FieldLabel htmlFor="app-health-enabled">
            {t('fields.healthEnable')}
          </FieldLabel>
        </Field>
        {healthCheckEnabled ? (
          <Field>
            <FieldLabel htmlFor="app-health-path">
              {t('fields.healthPath')}
            </FieldLabel>
            <Input
              id="app-health-path"
              className="font-mono"
              placeholder={HEALTH_CHECK_DEFAULT_PATH}
              {...register('healthCheckPath')}
            />
            <FieldHint>{t('fields.healthHint')}</FieldHint>
            <FieldError errors={[formState.errors.healthCheckPath]} />
          </Field>
        ) : null}
      </div>

      {multiNode ? (
        <Controller
          control={control}
          name="node"
          render={({ field }) => (
            <NodeSelectField
              idPrefix="app"
              nodes={nodes}
              withAuto
              value={field.value ?? AUTO_NODE_VALUE}
              onValueChange={field.onChange}
              error={[formState.errors.node]}
            />
          )}
        />
      ) : null}

      {nodes.length > 0 || projects.length > 0 ? (
        <button
          type="button"
          onClick={() => {
            setShowAdvanced((prev) => !prev)
          }}
          aria-expanded={showAdvanced}
          className="w-fit text-xs font-medium text-muted-foreground hover:text-foreground"
        >
          {showAdvanced ? t('fields.hideAdvanced') : t('fields.showAdvanced')}
        </button>
      ) : null}

      {showAdvanced ? (
        <div className="space-y-4 rounded-lg border border-dashed border-border p-3">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="app-strategy">
                {t('fields.strategy')}
              </FieldLabel>
              <Controller
                control={control}
                name="strategy"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="app-strategy" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={STRATEGY_DEFAULT_VALUE}>
                        {t('fields.strategyDefault')}
                      </SelectItem>
                      <SelectItem value="recreate">
                        {t('fields.strategyRecreate')}
                      </SelectItem>
                      <SelectItem value="blue-green">
                        {t('fields.strategyBlueGreen')}
                      </SelectItem>
                      <SelectItem value="rolling">
                        {t('fields.strategyRolling')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                )}
              />
              <FieldHint>{t('fields.strategyHint')}</FieldHint>
              <FieldError errors={[formState.errors.strategy]} />
            </Field>

            <Field>
              <FieldLabel htmlFor="app-replicas">
                {t('fields.replicas')}
              </FieldLabel>
              <Input
                id="app-replicas"
                type="number"
                step="1"
                min="1"
                placeholder="1"
                {...register('replicas')}
              />
              <FieldHint>{t('fields.replicasHint')}</FieldHint>
              <FieldError errors={[formState.errors.replicas]} />
            </Field>
          </div>

          {multiNode ? null : (
            <Controller
              control={control}
              name="node"
              render={({ field }) => (
                <NodeSelectField
                  idPrefix="app"
                  nodes={nodes}
                  value={field.value ?? LOCAL_NODE_VALUE}
                  onValueChange={field.onChange}
                  error={[formState.errors.node]}
                />
              )}
            />
          )}

          <Controller
            control={control}
            name="project"
            render={({ field }) => (
              <ProjectSelectField
                idPrefix="app"
                projects={projects}
                value={field.value ?? NO_PROJECT_VALUE}
                onValueChange={field.onChange}
                error={[formState.errors.project]}
              />
            )}
          />
        </div>
      ) : null}

      <NewAppPreflight
        request={{
          name: watch('name')?.trim() || undefined,
          image: watch('image')?.trim() || undefined,
          port: Number(watch('port')) || undefined,
        }}
      />

      {createApp.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
          <AlertDescription>{createApp.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={createApp.isPending}>
          {createApp.isPending ? t('fields.creating') : t('fields.create')}
        </Button>
      </DialogFooter>
    </CreateFormShell>
  )
}
