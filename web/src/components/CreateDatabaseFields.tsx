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

// The name/engine/version(/node) field set and its submit logic,
// factored out of what used to be CreateDatabaseDialog's own body so
// the exact same validation/mutation code backs both that standalone
// dialog and CreateResourceWizard's step 2 Postgres/Redis paths
// (docs/superpowers/specs/2026-08-14-creation-wizard-and-sidebar-design.md).
//
// Honesty note carried over from the original component's own doc
// comment: creating a postgres database always succeeds here, but the
// reconciler cannot actually start Postgres yet (no secrets management
// built). Postgres is not hidden or disabled as an option, since the
// backend genuinely accepts it; the real reconcile condition is visible
// on the detail page's status panel instead of being faked away here.
export function CreateDatabaseFields({
  open,
  onCreated,
  engine,
}: {
  /** The owning dialog's own open state, used only to reset this
   *  form's local state on close. See CreateAppFields's identical prop
   *  for the full reasoning (base-ui's Dialog keeps content mounted
   *  through its own closing animation, so unmount can't be relied on
   *  to clear stale values). */
  open: boolean
  /** Called once the database has been created and navigation to its
   *  detail page has been kicked off. */
  onCreated: () => void
  /** When set, the engine is fixed and the Select below isn't
   *  rendered: CreateResourceWizard's step 1 already asked "Postgres or
   *  Redis" as a separate card, so asking again via a dropdown in step
   *  2 would be the same question twice. When omitted (the standalone
   *  CreateDatabaseDialog's own usage), the Select is shown exactly as
   *  it always was, letting the one dialog cover both engines. */
  engine?: DatabaseEngine
}) {
  const navigate = useNavigate()
  const createDatabase = useCreateDatabase()
  const setDatabaseNode = useSetDatabaseNode()
  // Optional convenience only, see useNodeListOptional's own doc
  // comment: a failure or empty list here must never block database
  // creation, so the node field below is simply not rendered rather
  // than surfacing a loading or error state of its own.
  const nodeList = useNodeListOptional()
  const nodes = nodeList.data ?? []
  const defaultValues: CreateDatabaseFormInput = {
    name: '',
    engine: engine ?? 'postgres',
    version: '',
    node: LOCAL_NODE_VALUE,
  }
  const { control, register, handleSubmit, formState, reset, watch } = useForm<
    CreateDatabaseFormInput,
    unknown,
    CreateDatabaseFormOutput
  >({
    resolver: zodResolver(createDatabaseSchema),
    defaultValues,
  })
  const watchedEngine = watch('engine')

  useEffect(() => {
    if (!open) {
      reset(defaultValues)
      createDatabase.reset()
    }
    // Only reacting to the dialog's open transition, not to reset/
    // createDatabase/defaultValues identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

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
          onCreated()
          toast.add({
            title: `Database "${created.name}" created.`,
            type: 'success',
          })
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

      {engine ? null : (
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
      )}

      <Field>
        <FieldLabel htmlFor="database-version">Version</FieldLabel>
        <Input
          id="database-version"
          placeholder={VERSION_PLACEHOLDER[watchedEngine]}
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
          <WarningIcon />
          <AlertDescription>{createDatabase.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <DialogFooter>
        <Button type="submit" disabled={createDatabase.isPending}>
          {createDatabase.isPending ? 'Creating...' : 'Create database'}
        </Button>
      </DialogFooter>
    </form>
  )
}
