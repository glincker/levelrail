import { useEffect, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  UploadSimpleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { useDeployCompose } from '../queries/compose'

// Same app-name shape validateAppResource (internal/api/apps.go)
// ultimately enforces server-side; this is fast client-side feedback
// only, same reasoning createAppSchema's own comment (CreateAppFields)
// gives.
const APP_NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/

const createComposeSchema = z.object({
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

type FormInput = z.input<typeof createComposeSchema>
type FormOutput = z.output<typeof createComposeSchema>

const DEFAULT_VALUES: FormInput = { name: '', compose: '' }

const COMPOSE_PLACEHOLDER = `services:
  web:
    image: nginx:latest
    ports:
      - "80:80"`

// The "Docker Compose" step 2 path CreateResourceWizard adds: name plus
// a raw compose.yaml body, POSTed as-is to
// POST /api/v1/apps/{name}/compose (handleDeployCompose,
// queries/compose.ts's own doc comment covers why that's a raw-text
// body, not JSON). Unlike CreateAppFields/CreateAppFromGitFields, a
// successful submit doesn't just close the dialog: it swaps the form
// for a results panel listing every service the compose file fanned
// out into, since a compose deploy can create several apps at once and
// a plain "created" toast would hide that.
export function CreateComposeFields({
  open,
  onCreated,
}: {
  /** The owning dialog's own open state, used only to reset this
   *  form's local state on close. See CreateAppFields's identical prop
   *  for the full reasoning. */
  open: boolean
  /** Called once the operator dismisses the results panel (the "Done"
   *  button below), not immediately on a successful deploy: the results
   *  panel is the whole point of this component, so it stays visible
   *  until the operator is done reading it. */
  onCreated: () => void
}) {
  const deployCompose = useDeployCompose()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const { register, handleSubmit, formState, reset, setValue } = useForm<
    FormInput,
    unknown,
    FormOutput
  >({
    resolver: zodResolver(createComposeSchema),
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

  const onSubmit = handleSubmit((values) => {
    deployCompose.mutate({
      name: values.name.trim(),
      composeYaml: values.compose,
    })
  })

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    file
      .text()
      .then((text) => {
        setValue('compose', text, { shouldValidate: true })
        setFileError(null)
      })
      .catch(() => {
        setFileError(
          'Could not read that file. Paste the YAML directly instead.',
        )
      })
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

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      <Field>
        <FieldLabel htmlFor="compose-app-name">Name</FieldLabel>
        <Input
          id="compose-app-name"
          placeholder="e.g. my-stack"
          {...register('name')}
        />
        <FieldHint>
          Groups every service in this compose file under one app name.
        </FieldHint>
        <FieldError errors={[formState.errors.name]} />
      </Field>

      <Field>
        <div className="flex items-center justify-between">
          <FieldLabel htmlFor="compose-yaml">compose.yaml</FieldLabel>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              fileInputRef.current?.click()
            }}
          >
            <UploadSimpleIcon />
            Upload file
          </Button>
          <input
            ref={fileInputRef}
            type="file"
            accept=".yaml,.yml,text/yaml"
            className="hidden"
            onChange={handleFileChange}
          />
        </div>
        <Textarea
          id="compose-yaml"
          className="min-h-48 font-mono text-xs"
          placeholder={COMPOSE_PLACEHOLDER}
          spellCheck={false}
          {...register('compose')}
        />
        <FieldError errors={[formState.errors.compose]} />
        {fileError ? <FieldHint>{fileError}</FieldHint> : null}
      </Field>

      {deployCompose.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{deployCompose.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={deployCompose.isPending}>
          {deployCompose.isPending ? 'Deploying...' : 'Deploy'}
        </Button>
      </DialogFooter>
    </form>
  )
}
