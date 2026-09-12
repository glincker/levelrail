import { useEffect, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  InfoIcon,
  ShieldWarningIcon,
  UploadSimpleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { useDeployCompose } from '../queries/compose'
import { useFormDraft } from '../hooks/useFormDraft'
import { useIsRoot } from '../hooks/useIsRoot'
import { DraftRestoredNotice } from './DraftRestoredNotice'

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

// BindMountHelper builds one volumes: line for a bind mount (an absolute
// host path, not a Docker-managed named volume) and hands it to onInsert
// to append into the compose textarea, since this form otherwise has no
// structured place to put one: compose.yaml itself stays a single raw-
// text field (CreateComposeFields' own doc comment), so this is a
// generator for a line the operator pastes under the right service's
// volumes: list, not a field that submits on its own.
//
// Root-gated client-side via useIsRoot, mirroring routes/settings/
// users.tsx's own create-user/invite trigger gating: the real boundary
// is server-side (internal/api/apps_compose.go's handleDeployCompose
// requires AbilityRoot whenever the parsed compose body carries a bind
// mount, 403 otherwise), this only avoids showing a control that would
// just come back rejected for a non-root caller.
function BindMountHelper({ onInsert }: { onInsert: (line: string) => void }) {
  const isRoot = useIsRoot()
  const [hostPath, setHostPath] = useState('')
  const [containerPath, setContainerPath] = useState('')
  const [readOnly, setReadOnly] = useState(false)

  if (!isRoot) {
    return (
      <FieldHint>
        Bind-mounting a real host directory (unlike a Docker-managed named
        volume above) requires the root ability. Ask an admin for access if
        this service needs one.
      </FieldHint>
    )
  }

  const canInsert = hostPath.startsWith('/') && containerPath.startsWith('/')

  function handleInsert() {
    if (!canInsert) return
    onInsert(`      - "${hostPath}:${containerPath}${readOnly ? ':ro' : ''}"`)
    setHostPath('')
    setContainerPath('')
    setReadOnly(false)
  }

  return (
    <div className="space-y-3 rounded-lg border border-border p-3">
      <div className="flex items-center gap-2">
        <ShieldWarningIcon
          className="size-4 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <p className="text-sm font-medium text-foreground">
          Add a bind mount
        </p>
      </div>
      <Alert variant="destructive">
        <WarningIcon />
        <AlertDescription>
          This gives the container direct access to a real directory on the
          host machine it runs on, not a Docker-managed volume. Never point
          this at a system directory: paths like /, /etc, /root, /boot,
          /sys, /proc, /var/lib/docker, and /var/run (including the Docker
          socket) are always rejected, even for a root caller.
        </AlertDescription>
      </Alert>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="bind-mount-host-path">Host path</FieldLabel>
          <Input
            id="bind-mount-host-path"
            placeholder="/srv/myapp/data"
            value={hostPath}
            onChange={(e) => {
              setHostPath(e.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="bind-mount-container-path">
            Container path
          </FieldLabel>
          <Input
            id="bind-mount-container-path"
            placeholder="/data"
            value={containerPath}
            onChange={(e) => {
              setContainerPath(e.target.value)
            }}
          />
        </Field>
      </div>
      <Field orientation="horizontal">
        <Checkbox
          id="bind-mount-read-only"
          checked={readOnly}
          onCheckedChange={(checked) => {
            setReadOnly(checked === true)
          }}
        />
        <FieldLabel htmlFor="bind-mount-read-only">Read-only</FieldLabel>
      </Field>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canInsert}
          onClick={handleInsert}
        >
          Insert into compose.yaml
        </Button>
        {!canInsert && (hostPath || containerPath) ? (
          <span className="text-xs text-muted-foreground">
            Both paths must be absolute (start with &ldquo;/&rdquo;).
          </span>
        ) : null}
      </div>
    </div>
  )
}

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
  const {
    register,
    handleSubmit,
    formState,
    reset,
    setValue,
    getValues,
    watch,
  } = useForm<FormInput, unknown, FormOutput>({
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

  // Neither field is secret: name is an app-group name, compose is the
  // pasted/uploaded YAML body itself, exactly what the operator would
  // otherwise lose to a refresh or crash mid-paste.
  const { restoredFromDraft, discardDraft, dismissDraftNotice, clearDraft } =
    useFormDraft({
      storageKey: 'app-create-compose-draft',
      open,
      watch,
      reset,
      defaultValues: DEFAULT_VALUES,
    })

  const onSubmit = handleSubmit((values) => {
    deployCompose.mutate(
      {
        name: values.name.trim(),
        composeYaml: values.compose,
      },
      { onSuccess: clearDraft },
    )
  })

  // handleInsertBindMountLine appends a generated volumes: line
  // (BindMountHelper's own doc comment) to the end of whatever compose
  // YAML is already there, the same "no cursor-position tracking, just
  // append" approach handleFileChange takes for an uploaded file.
  function handleInsertBindMountLine(line: string) {
    const current = getValues('compose')
    const separator = current && !current.endsWith('\n') ? '\n' : ''
    setValue('compose', current + separator + line + '\n', {
      shouldValidate: true,
    })
  }

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
    const { app_id: appId, services, notices } = deployCompose.data
    return (
      <div className="space-y-4">
        <Alert>
          <CheckCircleIcon className="text-green-600 dark:text-green-400" />
          <AlertDescription>
            {services.length} {services.length === 1 ? 'service' : 'services'}{' '}
            deployed under &ldquo;{appId}&rdquo;.
          </AlertDescription>
        </Alert>
        {notices?.map((notice, i) => (
          <Alert key={i}>
            {notice.level === 'warning' ? <WarningIcon /> : <InfoIcon />}
            <AlertDescription>{notice.message}</AlertDescription>
          </Alert>
        ))}
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
      {restoredFromDraft ? (
        <DraftRestoredNotice
          onDiscard={discardDraft}
          onDismiss={dismissDraftNotice}
        />
      ) : null}

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

      <BindMountHelper onInsert={handleInsertBindMountLine} />

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
