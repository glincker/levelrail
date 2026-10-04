import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Link } from '@tanstack/react-router'
import {
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  CheckCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { formatBytes } from '../lib/format'
import { RamFitBadge } from './RamFitBadge'
import { ServiceTemplateGrid } from './ServiceTemplateGrid'
import { useDeployTemplateNow } from '../hooks/useDeployTemplateNow'
import { useDeployCompose } from '../queries/compose'
import {
  useServiceTemplate,
  type ServiceTemplateListItem,
} from '../queries/serviceTemplates'

// Same app-name shape CreateComposeFields validates against
// (validateAppResource, internal/api/apps.go).
const APP_NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

const deployTemplateSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .regex(
      APP_NAME_PATTERN,
      'Use lowercase letters, numbers, and hyphens only',
    ),
  compose: z.string().trim().min(1, 'A compose.yaml body is required'),
})

type FormInput = z.input<typeof deployTemplateSchema>
type FormOutput = z.output<typeof deployTemplateSchema>

const DEFAULT_VALUES: FormInput = { name: '', compose: '' }

// The "Browse templates" step 2 path CreateResourceWizard adds: a
// searchable grid over GET /api/v1/service-templates
// (queries/serviceTemplates.ts, internal/catalog's curated catalog, ADR
// 015), and once a template is picked, a preview/confirm step that
// fetches its full compose body from GET /api/v1/service-templates/{id}
// and pre-fills the exact same deploy form CreateComposeFields already
// owns: same useDeployCompose() mutation, same POST
// /api/v1/apps/{name}/compose request, same per-service results panel on
// success. Each card also has its own "Deploy now" action
// (handleDeployNow): for a template with no required, default-less
// secret this skips straight to POST /api/v1/service-templates/{id}/deploy
// and lands on the new app's own page; for one that does need
// configuration it falls back to this same preview/confirm step. Deploy
// semantics are not duplicated anywhere in this file.
export function BrowseTemplatesFields({
  open,
  onCreated,
  initialTemplateId,
  onViewChange,
}: {
  /** The owning dialog's own open state, used only to reset this
   *  component's local state on close. See CreateAppFields's identical
   *  prop for the full reasoning. */
  open: boolean
  /** Called once the operator dismisses the results panel, matching
   *  CreateComposeFields' own onCreated timing. */
  onCreated: () => void
  /** Skips the grid and opens straight to this template's preview/
   *  configure step, for a caller (the /templates/$id detail page's
   *  "Configure and deploy" button) that already knows which template it
   *  wants. Undefined keeps the normal grid-first behavior. */
  initialTemplateId?: string
  /** Reports which of this component's own two views is showing, so the
   *  owning wizard can widen its fullscreen layout for the wide catalog
   *  grid without also widening the narrow configure-form/results view. */
  onViewChange?: (view: 'grid' | 'detail') => void
}) {
  // Not reset via the `open` effect below: CreateResourceWizard's own
  // step-2 branch unmounts this component entirely when the dialog
  // closes (`selected` goes back to null), so a fresh open already gets
  // fresh useState calls here, the same reasoning CreateAppFields' own
  // showAdvanced state gives. Only react-hook-form/mutation state needs
  // the effect, since base-ui's Dialog keeps content mounted through its
  // own closing animation (see CreateAppFields' `open` prop doc comment).
  const [templateId, setTemplateId] = useState<string | null>(
    initialTemplateId ?? null,
  )
  const templateDetail = useServiceTemplate(templateId ?? '')
  const deployCompose = useDeployCompose()
  const deployTemplateNow = useDeployTemplateNow(onCreated)

  const { register, handleSubmit, formState, reset, setValue } = useForm<
    FormInput,
    unknown,
    FormOutput
  >({
    resolver: zodResolver(deployTemplateSchema),
    defaultValues: DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!open) {
      reset(DEFAULT_VALUES)
      deployCompose.reset()
    }
    // Only reacting to the dialog's open transition, not to reset/
    // deployCompose identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  useEffect(() => {
    onViewChange?.(templateId || deployCompose.isSuccess ? 'detail' : 'grid')
    // Only reacting to the two states that actually change which view
    // renders below, not to onViewChange identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateId, deployCompose.isSuccess])

  useEffect(() => {
    if (templateDetail.data) {
      setValue('name', templateDetail.data.id, { shouldValidate: true })
      setValue('compose', templateDetail.data.compose, {
        shouldValidate: true,
      })
    }
    // Only reacting to a freshly fetched template, not to setValue
    // identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templateDetail.data])

  const onSubmit = handleSubmit((values) => {
    deployCompose.mutate({
      name: values.name.trim(),
      composeYaml: values.compose,
    })
  })

  function backToGrid() {
    setTemplateId(null)
    deployCompose.reset()
  }

  // The genuine one-click path: a template with no required, default-
  // less secret (requires_configuration, computed server-side from its
  // Compose body, internal/api/service_templates.go's
  // templateRequiresConfiguration) deploys immediately under a
  // generated name via the shared useDeployTemplateNow hook (toast +
  // navigate to the new app's page). A template that does need
  // configuration falls back to the existing pre-filled step
  // (setTemplateId), the same screen a plain card click already opens,
  // rather than deploying with an empty or fake secret.
  function handleDeployNow(template: ServiceTemplateListItem) {
    if (template.requires_configuration) {
      setTemplateId(template.id)
      return
    }
    deployTemplateNow.deploy(template.id)
  }

  if (deployCompose.isSuccess) {
    const { app_id: appId, services } = deployCompose.data
    return (
      <div className="space-y-4">
        <Alert>
          <CheckCircleIcon className="text-green-600 dark:text-green-400" />
          <AlertDescription>
            {services.length} {services.length === 1 ? 'service' : 'services'}{' '}
            deployed under &ldquo;{appId}&rdquo;.
          </AlertDescription>
        </Alert>
        <ul className="space-y-2">
          {services.map((service) => (
            <li
              key={service.name}
              className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card p-2.5"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium text-foreground">
                    {service.name}
                  </span>
                  <Badge variant="success">Created</Badge>
                </div>
                <p className="truncate text-xs text-muted-foreground">
                  {service.image}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                render={
                  <Link to="/apps/$name" params={{ name: service.name }} />
                }
                nativeButton={false}
              >
                View
              </Button>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button type="button" onClick={onCreated}>
            Done
          </Button>
        </DialogFooter>
      </div>
    )
  }

  if (templateId) {
    return (
      <div className="space-y-4">
        <Button type="button" variant="ghost" size="sm" onClick={backToGrid}>
          <ArrowLeftIcon />
          Back to templates
        </Button>
        {templateDetail.isLoading ? (
          <div className="space-y-3">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-48 w-full" />
          </div>
        ) : templateDetail.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{templateDetail.error.message}</AlertDescription>
          </Alert>
        ) : templateDetail.data ? (
          <form
            onSubmit={(e) => {
              void onSubmit(e)
            }}
            className="space-y-4"
          >
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-sm font-medium text-foreground">
                  {templateDetail.data.name}
                </span>
                <Badge variant="outline">{templateDetail.data.category}</Badge>
                {!!templateDetail.data.recommended_memory_bytes && (
                  <>
                    <Badge variant="muted">
                      ~
                      {formatBytes(
                        templateDetail.data.recommended_memory_bytes,
                      )}{' '}
                      RAM recommended
                    </Badge>
                    <RamFitBadge
                      recommendedMemoryBytes={
                        templateDetail.data.recommended_memory_bytes
                      }
                    />
                  </>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {templateDetail.data.slogan}
              </p>
              {templateDetail.data.documentation_url ? (
                <a
                  href={templateDetail.data.documentation_url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="inline-flex items-center gap-1 text-xs text-primary underline underline-offset-4"
                >
                  Documentation
                  <ArrowSquareOutIcon className="size-3" aria-hidden="true" />
                </a>
              ) : null}
            </div>

            <Field>
              <FieldLabel htmlFor="template-app-name">Name</FieldLabel>
              <Input
                id="template-app-name"
                placeholder="e.g. my-stack"
                {...register('name')}
              />
              <FieldHint>
                Groups every service this template defines under one app name.
              </FieldHint>
              <FieldError errors={[formState.errors.name]} />
            </Field>

            <Field>
              <FieldLabel htmlFor="template-compose">compose.yaml</FieldLabel>
              <Textarea
                id="template-compose"
                className="min-h-48 font-mono text-xs"
                spellCheck={false}
                {...register('compose')}
              />
              <FieldHint>
                Pre-filled from the template. Edit it if you need to before
                deploying.
              </FieldHint>
              <FieldError errors={[formState.errors.compose]} />
            </Field>

            {deployCompose.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>
                  {deployCompose.error.message}
                </AlertDescription>
              </Alert>
            ) : null}

            <DialogFooter>
              <Button type="submit" disabled={deployCompose.isPending}>
                {deployCompose.isPending ? 'Deploying...' : 'Deploy'}
              </Button>
            </DialogFooter>
          </form>
        ) : null}
      </div>
    )
  }

  // No independent scroll region: the owning DialogContent is the single
  // scroll owner (dialog.tsx). A fixed max-h box here used to clip
  // whichever category header landed just above it.
  return (
    <ServiceTemplateGrid
      onSelect={setTemplateId}
      onDeployNow={handleDeployNow}
      deployingId={deployTemplateNow.pendingId}
    />
  )
}
