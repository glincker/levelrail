import { useEffect } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useCreateApp, useSetAppNode } from '../queries/apps'
import { useNodeListOptional } from '../queries/nodes'

// Sentinel for "this control plane's own local node", the implicit
// default PUT /api/v1/apps/{name}/node's own doc comment establishes
// (empty node_id). Not a real row in the node list, there is no
// separate "local" entry the API returns.
const LOCAL_NODE_VALUE = ''

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
const createAppSchema = z.object({
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
})

type CreateAppFormInput = z.input<typeof createAppSchema>
type CreateAppFormOutput = z.output<typeof createAppSchema>

const DEFAULT_VALUES: CreateAppFormInput = {
  name: '',
  image: '',
  port: '',
  node: LOCAL_NODE_VALUE,
}

// The name/image/port(/node) field set and its submit logic, factored
// out of what used to be CreateAppDialog's own body so the exact same
// validation/mutation code backs both that standalone dialog and
// CreateResourceWizard's step 2 "Docker image" path:
// one schema, one useCreateApp call, not copy-pasted a second time for
// the wizard. Only name/image/port are collected (the app spec's
// minimal essentials are build, domains, and port; domains/env/
// resources/health are real fields the backend accepts but out of
// scope here, a manually-created app via this form is the "already
// have a built image" path, distinct from and unaffected by the
// git-push spec path).
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
  const navigate = useNavigate()
  const createApp = useCreateApp()
  const setAppNode = useSetAppNode()
  // Optional convenience only, see useNodeListOptional's own doc
  // comment: a failure or empty list here must never block app
  // creation, so the node field below is simply not rendered rather
  // than surfacing a loading or error state of its own.
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  const { control, register, handleSubmit, formState, reset } = useForm<
    CreateAppFormInput,
    unknown,
    CreateAppFormOutput
  >({
    resolver: zodResolver(createAppSchema),
    defaultValues: DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!open) {
      reset(DEFAULT_VALUES)
      createApp.reset()
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createApp identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const onSubmit = handleSubmit((values) => {
    const nodeId = values.node ?? LOCAL_NODE_VALUE
    createApp.mutate(
      {
        name: values.name.trim(),
        image: values.image.trim(),
        port: values.port,
      },
      {
        onSuccess: (created) => {
          onCreated()
          toast.add({
            title: `App "${created.name}" created.`,
            type: 'success',
          })
          void navigate({
            to: '/apps/$name',
            params: { name: created.name },
          })
          // Placement is a trailing, best-effort call: the app row
          // already exists at this point, so a placement failure must
          // never look like the whole creation failed. Only fired when
          // a non-default node was actually picked.
          if (nodeId !== LOCAL_NODE_VALUE) {
            setAppNode.mutate({ name: created.name, nodeId })
          }
        },
      },
    )
  })

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      <Field>
        <FieldLabel htmlFor="app-name">Name</FieldLabel>
        <Input
          id="app-name"
          placeholder="e.g. marketing-site"
          {...register('name')}
        />
        <FieldError errors={[formState.errors.name]} />
      </Field>

      <Field>
        <FieldLabel htmlFor="app-image">Image</FieldLabel>
        <Input
          id="app-image"
          placeholder="e.g. ghcr.io/you/app:latest"
          {...register('image')}
        />
        <FieldError errors={[formState.errors.image]} />
      </Field>

      <Field>
        <FieldLabel htmlFor="app-port">Port</FieldLabel>
        <Input
          id="app-port"
          type="number"
          step="1"
          min="1"
          placeholder="e.g. 3000"
          {...register('port')}
        />
        <FieldError errors={[formState.errors.port]} />
      </Field>

      {nodes.length > 0 ? (
        <Field>
          <FieldLabel htmlFor="app-node">Node</FieldLabel>
          <Controller
            control={control}
            name="node"
            render={({ field }) => (
              <Select
                value={field.value ?? LOCAL_NODE_VALUE}
                onValueChange={field.onChange}
              >
                <SelectTrigger id="app-node" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={LOCAL_NODE_VALUE}>
                    This control plane (local)
                  </SelectItem>
                  {nodes.map((node) => (
                    <SelectItem key={node.id} value={node.id}>
                      {node.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
          <FieldError errors={[formState.errors.node]} />
        </Field>
      ) : null}

      {createApp.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{createApp.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={createApp.isPending}>
          {createApp.isPending ? 'Creating...' : 'Create app'}
        </Button>
      </DialogFooter>
    </form>
  )
}
