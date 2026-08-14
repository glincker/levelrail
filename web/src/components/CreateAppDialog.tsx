import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { PackageIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
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

// Create-via-dialog flow for a new app (POST /api/v1/apps), following
// CreateTokenDialog/CreateAlertRuleDialog's established shape: a
// trigger, a form inside DialogContent, mutate on submit. Unlike those
// two, the trigger itself is a prop rather than fixed inside this
// component: the apps list route needs the exact same dialog reachable
// from two places (the page header's "New app" button and the empty
// state's CTA), and this is the two-DialogTriggers-one-dialog-content
// pattern CreateTokenDialog's own doc comment gestures at, just with
// the trigger element supplied by each call site instead of hardcoded.
//
// Only name/image/port are collected (CLAUDE.md 4.9's minimal app
// spec: build, domains, port are the essentials; domains/env/resources/
// health are real fields the backend accepts but out of scope here, a
// manually-created app via this dialog is the "already have a built
// image" path, distinct from and unaffected by the git-push spec
// path). On success, the dialog closes and the user lands on the new
// app's detail page, the natural next step after creating something.
export function CreateAppDialog({ trigger }: { trigger: React.ReactElement }) {
  const [open, setOpen] = useState(false)
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

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(DEFAULT_VALUES)
      createApp.reset()
    }
  }

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
          handleOpenChange(false)
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
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <PackageIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            New app
          </DialogTitle>
          <DialogDescription>
            Register an app that already has a built image. Apps deployed from a
            git repo&apos;s app.yaml spec show up automatically instead.
          </DialogDescription>
        </DialogHeader>
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
      </DialogContent>
    </Dialog>
  )
}
