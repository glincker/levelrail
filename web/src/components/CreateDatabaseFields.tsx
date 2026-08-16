import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { Link, useNavigate } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { DialogFooter } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useCreateDatabase, useSetDatabaseNode } from '../queries/databases'
import { useDatabaseEnginesOptional } from '../queries/databaseEngines'
import { useNodeListOptional } from '../queries/nodes'
import { useProjectListOptional } from '../queries/projects'
import { useSystemStatusOptional } from '../queries/systemStatus'
import {
  LOCAL_NODE_VALUE,
  NO_PROJECT_VALUE,
  NodeSelectField,
  ProjectSelectField,
} from './PlacementFields'

// Engines whose reconciler branch (internal/reconcile/database/
// controller.go's Reconcile switch) refuses to start the container
// without generated credentials, reported as the CredentialsNotConfigured
// condition (ConditionsPanel.tsx's own reason-specific translation).
// Redis is deliberately excluded: it reconciles unconditionally, no
// secrets master key involved, see that controller's own package doc
// comment. Mirrors store.EnginePostgres/store.EngineMySQL by value
// rather than importing them, the same "plain non-empty string, not a
// literal union" reasoning createDatabaseSchema's own comment gives for
// why this file doesn't hardcode a second typed copy of the engine
// registry.
const CREDENTIALED_ENGINES = new Set(['postgres', 'mysql'])

// Presentation-only mapping to Docker Hub's official image slug for
// engine ids that have one, used solely to build the version field's
// "see available tags" link below. Not a source of truth for which
// engines exist (that's the /api/v1/database-engines registry this
// file already reads via useDatabaseEnginesOptional), so an id with no
// entry here just means the link is omitted, never a broken one.
const DOCKER_HUB_OFFICIAL_IMAGE: Record<string, string> = {
  postgres: 'postgres',
  mysql: 'mysql',
  redis: 'redis',
}

// Matches validateDatabaseResource (internal/api/databases.go): name/
// engine/version all required. engine is a plain non-empty string, not
// a fixed literal union: the real set of valid engines lives in
// internal/store/database_engines.yaml (GET /api/v1/database-engines,
// see queries/databaseEngines.ts), so this schema deliberately doesn't
// hardcode a second copy of that list just to type-check a string. This
// is client-side fast feedback only either way, same reasoning
// createAppSchema's own comment gives: the real request still goes out
// on submit and the real response (including a 409 on a duplicate name,
// or an unrecognized engine) is what onSubmit below shows.
const createDatabaseSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  engine: z.string().trim().min(1, 'Engine is required'),
  version: z.string().trim().min(1, 'Version is required'),
  // Optional target node id, empty string means the local node. Only
  // ever populated from the Select below, which only ever offers real
  // node ids or the local sentinel, so no further validation is needed
  // beyond what the Select already constrains it to.
  node: z.string().optional(),
  // Optional project id, same shape as node above.
  project: z.string().optional(),
})

type CreateDatabaseFormInput = z.input<typeof createDatabaseSchema>
type CreateDatabaseFormOutput = z.output<typeof createDatabaseSchema>

// The name/engine/version(/node) field set and its submit logic,
// factored out of what used to be CreateDatabaseDialog's own body so
// the exact same validation/mutation code backs both that standalone
// dialog and CreateResourceWizard's step 2 Postgres/Redis paths.
//
// Honesty note carried over from the original component's own doc
// comment, updated now that secrets management (TASKS.md 1.7) exists:
// creating a postgres or mysql database always succeeds here, but the
// reconciler still refuses to start either one until the control plane
// has a secrets master key configured (internal/reconcile/database/
// controller.go's credentialsBlockedResult). Neither engine is hidden or
// disabled as an option, since the backend genuinely accepts the
// desired state either way; the inline warning below surfaces the risk
// up front instead of only after the fact on the detail page's status
// panel (ConditionsPanel.tsx's own CredentialsNotConfigured
// translation), and creation is never blocked on it, matching this
// codebase's level-triggered "create now, converges later" design (see
// the database controller's own package doc comment: 4.2 in the root
// plan document, reconcilers are idempotent and safe to interrupt, never
// a reason to gate creation on a fixable operator-side prerequisite).
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
   *  rendered: CreateResourceWizard's step 1 already asked which engine
   *  as a separate card, so asking again via a dropdown in step 2 would
   *  be the same question twice. When omitted (the standalone
   *  CreateDatabaseDialog's own usage), the Select is shown exactly as
   *  it always was, letting the one dialog cover every registered
   *  engine. */
  engine?: string
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
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: a failure or empty list here must never block database
  // creation.
  const projectList = useProjectListOptional()
  const projects = projectList.data ?? []
  // Same optional-convenience treatment for the engine registry: when
  // `engine` is already fixed by the caller (the wizard's step 1), this
  // list is only consulted for the version placeholder below, not for
  // validity, so a slow/failed fetch degrades to "no placeholder
  // suggested" rather than blocking the form.
  const engineList = useDatabaseEnginesOptional()
  const engines = engineList.data ?? []
  // Optional convenience, same degrade-gracefully treatment as the
  // lists above: see useSystemStatusOptional's own doc comment
  // (queries/systemStatus.ts). Only ever used to decide whether to show
  // the credentials warning below, never to gate submission.
  const systemStatus = useSystemStatusOptional()
  // Node/project placement is a real option but not one a first-time
  // database needs, so it starts collapsed. Not reset on close, same
  // reasoning as CreateAppFields.tsx's identical toggle.
  const [showAdvanced, setShowAdvanced] = useState(false)
  const defaultValues: CreateDatabaseFormInput = {
    name: '',
    engine: engine ?? engines[0]?.id ?? '',
    version: '',
    node: LOCAL_NODE_VALUE,
    project: NO_PROJECT_VALUE,
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
        // Sent directly, same "safe at create time" reasoning
        // CreateAppFields' own onSubmit comment gives.
        project_id:
          values.project === NO_PROJECT_VALUE || !values.project
            ? undefined
            : values.project,
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
      {CREDENTIALED_ENGINES.has(watchedEngine) &&
      systemStatus.data?.secrets_configured === false ? (
        // Warning, not a gate: submission below is never disabled on
        // this. See the "Honesty note" comment above for why creating
        // now and converging once the operator fixes the master key is
        // the intended behavior here, not a bug to work around. No
        // "warning" tone exists in badgeVariants/alertVariants, so this
        // reuses the same amber-precedent classes AddNodeDialog.tsx and
        // CreateTokenDialog.tsx already established for exactly this
        // gap.
        <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
          <WarningIcon className="mt-0.5 size-4 shrink-0" />
          <p className="text-sm">
            No secrets master key is configured on this control plane.
            This database will be created but won&rsquo;t start until one
            is configured.{' '}
            <Link
              to="/settings/general"
              className="underline underline-offset-2"
            >
              Configure it in Settings &gt; General
            </Link>
            .
          </p>
        </div>
      ) : null}
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
                  {engines.map((option) => (
                    <SelectItem key={option.id} value={option.id}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
          <FieldHint>
            Postgres or MySQL suit relational data that needs joins and
            transactions. Redis has no credentials to configure and suits
            caching, queues, and other ephemeral state.
          </FieldHint>
          <FieldError errors={[formState.errors.engine]} />
        </Field>
      )}

      <Field>
        <FieldLabel htmlFor="database-version">Version</FieldLabel>
        <Input
          id="database-version"
          placeholder={
            engines.find((e) => e.id === watchedEngine)?.default_version
          }
          {...register('version')}
        />
        {DOCKER_HUB_OFFICIAL_IMAGE[watchedEngine] ? (
          <FieldHint
            href={`https://hub.docker.com/_/${DOCKER_HUB_OFFICIAL_IMAGE[watchedEngine]}/tags`}
            linkText="See available versions"
          >
            A major version like &quot;16&quot;, matching an image tag that
            actually exists.
          </FieldHint>
        ) : null}
        <FieldError errors={[formState.errors.version]} />
      </Field>

      {nodes.length > 0 || projects.length > 0 ? (
        <button
          type="button"
          onClick={() => {
            setShowAdvanced((prev) => !prev)
          }}
          aria-expanded={showAdvanced}
          className="w-fit text-xs font-medium text-muted-foreground hover:text-foreground"
        >
          {showAdvanced ? 'Hide advanced options' : 'Show advanced options'}
        </button>
      ) : null}

      {showAdvanced ? (
        <div className="space-y-4 rounded-lg border border-dashed border-border p-3">
          <Controller
            control={control}
            name="node"
            render={({ field }) => (
              <NodeSelectField
                idPrefix="database"
                nodes={nodes}
                value={field.value ?? LOCAL_NODE_VALUE}
                onValueChange={field.onChange}
                error={[formState.errors.node]}
              />
            )}
          />

          <Controller
            control={control}
            name="project"
            render={({ field }) => (
              <ProjectSelectField
                idPrefix="database"
                projects={projects}
                value={field.value ?? NO_PROJECT_VALUE}
                onValueChange={field.onChange}
                error={[formState.errors.project]}
              />
            )}
          />
        </div>
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
