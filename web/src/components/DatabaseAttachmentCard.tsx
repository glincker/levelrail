import { useState } from 'react'
import {
  DatabaseIcon,
  PlugsConnectedIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useSetAppDatabase, useClearAppDatabase } from '../queries/apps'
import { useDatabases } from '../queries/databases'
import type { AppDetail } from '../types/appDetail'

// Attach/detach card for PUT/DELETE /api/v1/apps/{name}/database
// (internal/api/apps_database.go): lets an app claim an existing managed
// database as a connection env var source without hand-editing app.yaml,
// the same "attach a resource, credentials just show up as an env var"
// story StorageAttachmentCard already tells for object storage, applied
// to a database (feature parity baseline this platform targets).
//
// Field defaults to "url" (a full connection string) and env var
// defaults to "DATABASE_URL", the common case; the "more fields" toggle
// exposes the per-field variants (host/port/username/password/database)
// and a custom env var name for the less common case.
const fieldOptions = [
  { value: 'url', label: 'Full connection URL' },
  { value: 'host', label: 'Host' },
  { value: 'port', label: 'Port' },
  { value: 'username', label: 'Username' },
  { value: 'password', label: 'Password' },
  { value: 'database', label: 'Database name' },
]

export function DatabaseAttachmentCard({ app }: { app: AppDetail }) {
  const [selectedDatabase, setSelectedDatabase] = useState('')
  const [envVar, setEnvVar] = useState('DATABASE_URL')
  const [field, setField] = useState('url')
  const [showAdvanced, setShowAdvanced] = useState(false)

  const databasesQuery = useDatabases()
  const databases = databasesQuery.data ?? []
  const setAppDatabase = useSetAppDatabase()
  const clearAppDatabase = useClearAppDatabase()
  const pending = setAppDatabase.isPending || clearAppDatabase.isPending

  const attachment = app.database_attachment
  const attachedDatabase = databases.find(
    (d) => d.name === attachment?.database_name,
  )

  const collidingEnvKey =
    !attachment && app.env?.[envVar] !== undefined ? envVar : undefined

  function handleAttach() {
    if (!selectedDatabase) {
      return
    }
    setAppDatabase.mutate(
      {
        name: app.name,
        databaseName: selectedDatabase,
        envVar: envVar || undefined,
        field: field || undefined,
      },
      {
        onSuccess: () => {
          setSelectedDatabase('')
          toast.add({
            title: 'Database attached.',
            description: `${app.name} now gets its connection value as ${envVar || 'DATABASE_URL'}.`,
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not attach database.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleDetach() {
    clearAppDatabase.mutate(app.name, {
      onError: (error) => {
        toast.add({
          title: 'Could not detach database.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Database</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {attachment ? (
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-start gap-2">
              <PlugsConnectedIcon
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <div>
                <p className="text-sm font-medium text-foreground">
                  {attachedDatabase?.name ?? attachment.database_name}
                </p>
                <p className="text-sm text-muted-foreground">
                  Injected as{' '}
                  <code className="font-mono">{attachment.env_var}</code> (
                  {attachment.field}) at container start.
                </p>
              </div>
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={pending}
              onClick={handleDetach}
            >
              {clearAppDatabase.isPending ? 'Detaching...' : 'Detach'}
            </Button>
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">
              No database attached. Attach an existing managed database to
              inject its connection value as an env var, no manual
              host/port/password copy-paste needed.
            </p>
            {databases.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No managed databases yet. Create one from the Databases page
                first.
              </p>
            ) : (
              <Field>
                <FieldLabel htmlFor="database-attach-select">
                  Database
                </FieldLabel>
                {collidingEnvKey ? (
                  <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
                    <WarningIcon className="mt-0.5 size-4 shrink-0" />
                    <p className="text-sm">
                      This app already has an env var named{' '}
                      <code className="font-mono">{collidingEnvKey}</code>.
                      Attaching this database will silently override it at
                      container start.
                    </p>
                  </div>
                ) : null}
                <div className="flex items-center gap-2">
                  <Select
                    value={selectedDatabase}
                    onValueChange={(value) => {
                      if (value) {
                        setSelectedDatabase(value)
                      }
                    }}
                  >
                    <SelectTrigger
                      id="database-attach-select"
                      className="w-full"
                    >
                      <SelectValue placeholder="Choose a managed database" />
                    </SelectTrigger>
                    <SelectContent>
                      {databases.map((d) => (
                        <SelectItem key={d.name} value={d.name}>
                          {d.name} ({d.engine})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    size="sm"
                    disabled={pending || !selectedDatabase}
                    onClick={handleAttach}
                  >
                    <DatabaseIcon className="size-3.5" aria-hidden="true" />
                    {setAppDatabase.isPending ? 'Attaching...' : 'Attach'}
                  </Button>
                </div>
                <button
                  type="button"
                  className="text-left text-sm text-muted-foreground underline-offset-2 hover:underline"
                  onClick={() => setShowAdvanced((v) => !v)}
                >
                  {showAdvanced ? 'Hide options' : 'Env var / field options'}
                </button>
                {showAdvanced ? (
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="database-attach-env-var">
                        Env var name
                      </FieldLabel>
                      <Input
                        id="database-attach-env-var"
                        value={envVar}
                        onChange={(e) => setEnvVar(e.target.value)}
                        placeholder="DATABASE_URL"
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="database-attach-field">
                        Field
                      </FieldLabel>
                      <Select
                        value={field}
                        onValueChange={(value) => {
                          if (value) {
                            setField(value)
                          }
                        }}
                      >
                        <SelectTrigger id="database-attach-field">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {fieldOptions.map((opt) => (
                            <SelectItem key={opt.value} value={opt.value}>
                              {opt.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>
                ) : null}
                <FieldDescription>
                  Matches app.yaml&apos;s own{' '}
                  <code className="font-mono">
                    {'{ from: "<database>.<field>" }'}
                  </code>{' '}
                  env var syntax; this is the equivalent for an app not
                  hand-editing a spec file.
                </FieldDescription>
              </Field>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
