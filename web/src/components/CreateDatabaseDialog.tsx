import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { DatabaseIcon, TriangleAlertIcon } from 'lucide-react'
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
import { useCreateDatabase, useSetDatabaseNode } from '../queries/databases'
import { useNodeListOptional } from '../queries/nodes'
import type { DatabaseEngine } from '../types/databaseDetail'

// Sentinel for "this control plane's own local node", the implicit
// default PUT /api/v1/databases/{name}/node's own doc comment
// establishes (empty node_id). Not a real row in the node list, there
// is no separate "local" entry the API returns.
const LOCAL_NODE_VALUE = ''

// Matches validateDatabaseResource (internal/api/databases.go) exactly:
// name/engine/version all required, engine must be exactly "postgres" or
// "redis". This is client-side fast feedback only, same reasoning
// createAppSchema's own comment gives: the real request still goes out
// on submit and the real response (including a 409 on a duplicate name)
// is what onSubmit below shows.
const createDatabaseSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  engine: z.enum(['postgres', 'redis'], { error: 'Engine is required' }),
  version: z.string().trim().min(1, 'Version is required'),
  // Optional target node id, empty string means the local node. Only
  // ever populated from the Select below, which only ever offers real
  // node ids or the local sentinel, so no further validation is needed
  // beyond what the Select already constrains it to.
  node: z.string().optional(),
})

type CreateDatabaseFormInput = z.input<typeof createDatabaseSchema>
type CreateDatabaseFormOutput = z.output<typeof createDatabaseSchema>

const DEFAULT_VALUES: CreateDatabaseFormInput = {
  name: '',
  engine: 'postgres',
  version: '',
  node: LOCAL_NODE_VALUE,
}

const ENGINE_OPTIONS: { value: DatabaseEngine; label: string }[] = [
  { value: 'postgres', label: 'Postgres' },
  { value: 'redis', label: 'Redis' },
]

// Version placeholder differs by engine so the field always suggests a
// realistic value: "16" reads oddly as a Redis version, "7" reads oddly
// as a Postgres one.
const VERSION_PLACEHOLDER: Record<DatabaseEngine, string> = {
  postgres: '16',
  redis: '7',
}

// Create-via-dialog flow for a new database (POST /api/v1/databases),
// following CreateAppDialog's shape almost exactly: a trigger supplied
// by the caller rather than fixed inside this component (the databases
// list route needs the same dialog reachable from both the page
// header's "New database" button and the empty state's CTA), mutate on
// submit, navigate to the new resource's detail page on success.
//
// Honesty note carried over from handleCreateDatabase's own doc
// comment: creating a postgres database always succeeds here, but the
// reconciler cannot actually start Postgres yet (no secrets management
// built). Postgres is not hidden or disabled as an option, since the
// backend genuinely accepts it; the real reconcile condition is visible
// on the detail page's status panel instead of being faked away here.
export function CreateDatabaseDialog({
  trigger,
}: {
  trigger: React.ReactElement
}) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const createDatabase = useCreateDatabase()
  const setDatabaseNode = useSetDatabaseNode()
  // Optional convenience only, see useNodeListOptional's own doc
  // comment: a failure or empty list here must never block database
  // creation, so the node field below is simply not rendered rather
  // than surfacing a loading or error state of its own.
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  const { control, register, handleSubmit, formState, reset, watch } = useForm<
    CreateDatabaseFormInput,
    unknown,
    CreateDatabaseFormOutput
  >({
    resolver: zodResolver(createDatabaseSchema),
    defaultValues: DEFAULT_VALUES,
  })
  const engine = watch('engine')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(DEFAULT_VALUES)
      createDatabase.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const nodeId = values.node ?? LOCAL_NODE_VALUE
    createDatabase.mutate(
      {
        name: values.name.trim(),
        engine: values.engine,
        version: values.version.trim(),
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          void navigate({
            to: '/databases/$name',
            params: { name: created.name },
          })
          // Placement is a trailing, best-effort call: the database row
          // already exists at this point, so a placement failure must
          // never look like the whole creation failed. Only fired when
          // a non-default node was actually picked.
          if (nodeId !== LOCAL_NODE_VALUE) {
            setDatabaseNode.mutate({ name: created.name, nodeId })
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
            <DatabaseIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            New database
          </DialogTitle>
          <DialogDescription>
            Register a managed Postgres or Redis database. The reconciler
            provisions it on the target node.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="database-name">Name</FieldLabel>
            <Input
              id="database-name"
              placeholder="e.g. main"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="database-engine">Engine</FieldLabel>
            <Controller
              control={control}
              name="engine"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="database-engine" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ENGINE_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            <FieldError errors={[formState.errors.engine]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="database-version">Version</FieldLabel>
            <Input
              id="database-version"
              placeholder={VERSION_PLACEHOLDER[engine]}
              {...register('version')}
            />
            <FieldError errors={[formState.errors.version]} />
          </Field>

          {nodes.length > 0 ? (
            <Field>
              <FieldLabel htmlFor="database-node">Node</FieldLabel>
              <Controller
                control={control}
                name="node"
                render={({ field }) => (
                  <Select
                    value={field.value ?? LOCAL_NODE_VALUE}
                    onValueChange={field.onChange}
                  >
                    <SelectTrigger id="database-node" className="w-full">
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

          {createDatabase.isError ? (
            <Alert variant="destructive">
              <TriangleAlertIcon />
              <AlertDescription>
                {createDatabase.error.message}
              </AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createDatabase.isPending}>
              {createDatabase.isPending ? 'Creating...' : 'Create database'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
