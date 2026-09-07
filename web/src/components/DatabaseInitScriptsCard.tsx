import { useState } from 'react'
import type { ReactElement } from 'react'
import {
  CodeIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import {
  useCreateDatabaseInitScript,
  useDatabaseInitScripts,
  useDeleteDatabaseInitScript,
  useUpdateDatabaseInitScript,
} from '../queries/databaseInitScripts'
import type { DatabaseInitScript } from '../types/databaseInitScript'

// Init scripts (postgres/mysql/mariadb/mongodb only): named SQL/shell
// files mounted into the database's container at
// /docker-entrypoint-initdb.d, run once on first container start
// against an empty data volume. Editing an existing database's scripts
// has no effect until it's recreated from scratch, Docker's own
// documented behavior for that directory, not something this platform
// can override, so the card says so plainly rather than implying a
// live update.
const SUPPORTED_ENGINES = new Set(['postgres', 'mysql', 'mariadb', 'mongodb'])

function extensionHintFor(engine: string): string {
  return engine === 'mongodb' ? '.js or .sh' : '.sql or .sh'
}

function ScriptFormDialog({
  databaseName,
  engine,
  script,
  trigger,
}: {
  databaseName: string
  engine: string
  script?: DatabaseInitScript
  trigger: ReactElement
}) {
  const isEdit = script !== undefined
  const [open, setOpen] = useState(false)
  const [filename, setFilename] = useState(script?.filename ?? '')
  const [content, setContent] = useState(script?.content ?? '')
  const createScript = useCreateDatabaseInitScript(databaseName)
  const updateScript = useUpdateDatabaseInitScript(databaseName)
  const mutation = isEdit ? updateScript : createScript

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setFilename(script?.filename ?? '')
      setContent(script?.content ?? '')
      mutation.reset()
    }
  }

  const canSubmit = filename.trim().length > 0 && content.trim().length > 0

  function handleSubmit() {
    const req = { filename: filename.trim(), content }
    const onSuccess = () => {
      setOpen(false)
      toast.add({
        title: isEdit
          ? `Script "${req.filename}" updated.`
          : `Script "${req.filename}" added.`,
        description:
          'Takes effect the next time this database is created against an empty data volume, not retroactively.',
        type: 'success',
      })
    }
    const onError = (error: Error) => {
      toast.add({
        title: isEdit ? 'Could not update script.' : 'Could not add script.',
        description: error.message,
        type: 'error',
      })
    }
    if (isEdit) {
      updateScript.mutate({ id: script.id, req }, { onSuccess, onError })
    } else {
      createScript.mutate(req, { onSuccess, onError })
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? 'Edit init script' : 'Add init script'}</DialogTitle>
          <DialogDescription>
            Runs once, the next time this database&apos;s container starts
            against an empty data volume. Editing an already-initialized
            database&apos;s scripts has no effect unless it&apos;s recreated
            from scratch.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="init-script-filename">Filename</FieldLabel>
          <Input
            id="init-script-filename"
            placeholder={`e.g. 01-extensions${extensionHintFor(engine)}`}
            autoComplete="off"
            spellCheck={false}
            value={filename}
            onChange={(event) => {
              setFilename(event.target.value)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="init-script-content">Content</FieldLabel>
          <Textarea
            id="init-script-content"
            rows={10}
            className="font-mono text-xs"
            spellCheck={false}
            value={content}
            onChange={(event) => {
              setContent(event.target.value)
            }}
          />
        </Field>
        {mutation.isError ? (
          <p className="text-sm text-destructive">{mutation.error.message}</p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!canSubmit || mutation.isPending}
            onClick={handleSubmit}
          >
            {mutation.isPending ? 'Saving...' : isEdit ? 'Save changes' : 'Add script'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function DeleteScriptDialog({
  databaseName,
  script,
}: {
  databaseName: string
  script: DatabaseInitScript
}) {
  const [open, setOpen] = useState(false)
  const deleteScript = useDeleteDatabaseInitScript(databaseName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteScript.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="ghost" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{script.filename}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This only removes it from what a future (re)created database
            would run. It has no effect on this database&apos;s already-running
            container.
          </DialogDescription>
        </DialogHeader>
        {deleteScript.isError ? (
          <p className="text-sm text-destructive">{deleteScript.error.message}</p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteScript.isPending}
            onClick={() => {
              deleteScript.mutate(script.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: `Script "${script.filename}" deleted.`, type: 'success' })
                },
                onError: (error) => {
                  toast.add({
                    title: 'Could not delete script.',
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteScript.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ScriptRow({
  databaseName,
  engine,
  script,
}: {
  databaseName: string
  engine: string
  script: DatabaseInitScript
}) {
  return (
    <div className="flex items-center justify-between gap-3 py-2.5 text-sm">
      <p className="min-w-0 truncate font-mono text-foreground">{script.filename}</p>
      <div className="flex shrink-0 items-center gap-1.5">
        <ScriptFormDialog
          databaseName={databaseName}
          engine={engine}
          script={script}
          trigger={
            <Button variant="ghost" size="sm">
              <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
            </Button>
          }
        />
        <DeleteScriptDialog databaseName={databaseName} script={script} />
      </div>
    </div>
  )
}

export function DatabaseInitScriptsCard({
  databaseName,
  engine,
}: {
  databaseName: string
  engine: string
}) {
  const { data, isLoading } = useDatabaseInitScripts(databaseName)

  if (!SUPPORTED_ENGINES.has(engine)) {
    return null
  }

  const scripts = data ?? []

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
        <div>
          <CardTitle>Init scripts</CardTitle>
          <CardDescription>
            Runs once, the first time this database starts against an empty
            data volume.
          </CardDescription>
        </div>
        <ScriptFormDialog
          databaseName={databaseName}
          engine={engine}
          trigger={
            <Button variant="outline" size="sm">
              <PlusIcon className="size-3.5" aria-hidden="true" />
              Add script
            </Button>
          }
        />
      </CardHeader>
      <CardContent>
        {isLoading ? null : scripts.length === 0 ? (
          <EmptyState
            icon={<CodeIcon className="size-5" />}
            title="No init scripts"
            description="Add a SQL or shell file to run once when this database's container first starts, e.g. CREATE EXTENSION for pgvector."
          />
        ) : (
          <div className="divide-y divide-border">
            {scripts.map((script) => (
              <ScriptRow
                key={script.id}
                databaseName={databaseName}
                engine={engine}
                script={script}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
