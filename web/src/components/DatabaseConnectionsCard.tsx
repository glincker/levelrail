import { useState } from 'react'
import {
  DatabaseIcon,
  HardDrivesIcon,
  PlugsConnectedIcon,
  ShareNetworkIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
import { DisconnectConnectionDialog } from './ConnectionCard'
import {
  useAppConnections,
  useConnectableDatabases,
  useCreateAppConnection,
  useDeleteAppConnection,
  type AppConnection,
} from '../queries/appConnections'
import type { AppDetail } from '../types/appDetail'

// "Connections" section for the app detail page: the DatabaseEnv-map-
// backed sibling of DatabaseAttachmentCard's single-slot "Database"
// section, letting an app connect to more than one managed database
// (e.g. its own Postgres plus a shared Redis cache) without hand-editing
// app.yaml. Every entry here is system-managed (injected by the
// reconciler at container start, POST/GET/DELETE
// /api/v1/apps/{name}/connections), never something an operator should
// hand-edit in the Env editor: the badge on each row makes that
// distinction visible so nobody is surprised when a manual edit to the
// same key gets silently overwritten at the next deploy.
const fieldOptions = [
  { value: 'url', label: 'Full connection URL' },
  { value: 'host', label: 'Host' },
  { value: 'port', label: 'Port' },
  { value: 'username', label: 'Username' },
  { value: 'password', label: 'Password' },
  { value: 'database', label: 'Database name' },
]

function ConnectionReachabilityBadge({
  connection,
}: Readonly<{ connection: AppConnection }>) {
  if (!connection.cross_node) {
    return <Badge variant="outline">Same node</Badge>
  }
  if (connection.mesh_dns) {
    return (
      <Badge variant="success">
        <ShareNetworkIcon className="size-3" aria-hidden="true" />
        Cross-node (mesh DNS)
      </Badge>
    )
  }
  return (
    <Badge variant="destructive">
      <WarningIcon className="size-3" aria-hidden="true" />
      Cross-node, mesh disabled
    </Badge>
  )
}

function ConnectionRow({
  connection,
  onDisconnect,
  pending,
}: Readonly<{
  connection: AppConnection
  onDisconnect: (envVar: string) => void
  pending: boolean
}>) {
  const [confirmOpen, setConfirmOpen] = useState(false)

  return (
    <div className="flex items-start justify-between gap-4 rounded-lg border border-border p-3">
      <div className="flex items-start gap-2">
        <PlugsConnectedIcon
          className="mt-0.5 size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <code className="font-mono text-sm font-medium text-foreground">
              {connection.env_var}
            </code>
            <Badge variant="muted">System-managed</Badge>
            <ConnectionReachabilityBadge connection={connection} />
          </div>
          <p className="text-sm text-muted-foreground">
            {connection.database_name} ({connection.field}) resolves to{' '}
            <code className="font-mono">{connection.host}</code>
          </p>
          {connection.cross_node && !connection.mesh_dns ? (
            <p className="text-sm text-destructive">
              This database is on a different node and mesh networking
              isn&apos;t configured on this control plane, so this connection
              will not reach it. Enable mesh networking, or move one of them to
              the same node.
            </p>
          ) : null}
        </div>
      </div>
      <DisconnectConnectionDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Remove this connection?"
        description={
          <>
            The app stops getting{' '}
            <code className="font-mono">{connection.env_var}</code> injected at
            its next container start. The database itself is not affected.
          </>
        }
        pending={pending}
        actionLabel="Remove"
        pendingLabel="Removing..."
        onConfirm={() => {
          onDisconnect(connection.env_var)
          setConfirmOpen(false)
        }}
      />
    </div>
  )
}

export function DatabaseConnectionsCard({ app }: { app: AppDetail }) {
  const [selectedDatabase, setSelectedDatabase] = useState('')
  const [field, setField] = useState('url')
  const [envVar, setEnvVar] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)

  const connectionsQuery = useAppConnections(app.name)
  const connectableQuery = useConnectableDatabases(app.name)
  const connections = connectionsQuery.data ?? []
  const candidates = connectableQuery.data ?? []

  const createConnection = useCreateAppConnection(app.name)
  const deleteConnection = useDeleteAppConnection(app.name)

  function handleConnect() {
    if (!selectedDatabase) {
      return
    }
    createConnection.mutate(
      { database: selectedDatabase, field, envVar: envVar || undefined },
      {
        onSuccess: (result) => {
          setSelectedDatabase('')
          setEnvVar('')
          setShowAdvanced(false)
          toast.add({
            title: 'Database connected.',
            description: `${app.name} now gets ${result.database_name} as ${result.env_var}.`,
            type: 'success',
          })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not connect database.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  function handleDisconnect(targetEnvVar: string) {
    deleteConnection.mutate(targetEnvVar, {
      onSuccess: () => {
        toast.add({
          title: 'Connection removed.',
          description: `${app.name} no longer gets ${targetEnvVar} injected.`,
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not remove connection.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const suggestions = candidates.filter((d) => !d.already_connected)

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connections</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {connections.length > 0 ? (
          <div className="space-y-2">
            {connections.map((c) => (
              <ConnectionRow
                key={c.env_var}
                connection={c}
                onDisconnect={handleDisconnect}
                pending={
                  deleteConnection.isPending &&
                  deleteConnection.variables === c.env_var
                }
              />
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            No database connections yet. Connect a managed database below to
            inject its connection value as an env var, no manual host/port/
            password copy-paste needed. Unlike the legacy single-database
            attachment, an app can have any number of these.
          </p>
        )}

        <div className="space-y-3 border-t border-border pt-4">
          {candidates.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No managed databases yet. Create one from the Databases page
              first.
            </p>
          ) : (
            <Field>
              <FieldLabel htmlFor="connection-database-select">
                Connect a database
              </FieldLabel>
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
                    id="connection-database-select"
                    className="w-full"
                  >
                    <SelectValue placeholder="Choose a managed database" />
                  </SelectTrigger>
                  <SelectContent>
                    {candidates.map((d) => (
                      <SelectItem key={d.name} value={d.name}>
                        <span className="flex items-center gap-1.5">
                          {d.name} ({d.engine})
                          {d.cross_node ? (
                            <HardDrivesIcon
                              className="size-3.5 text-muted-foreground"
                              aria-hidden="true"
                            />
                          ) : null}
                        </span>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  size="sm"
                  disabled={createConnection.isPending || !selectedDatabase}
                  onClick={handleConnect}
                >
                  <DatabaseIcon className="size-3.5" aria-hidden="true" />
                  {createConnection.isPending ? 'Connecting...' : 'Connect'}
                </Button>
              </div>
              {suggestions.length > 0 && !selectedDatabase ? (
                <FieldDescription>
                  Suggested: {suggestions.map((d) => d.name).join(', ')}
                  {suggestions.some((d) => d.cross_node)
                    ? ' (some are on a different node)'
                    : ''}
                </FieldDescription>
              ) : null}
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
                    <FieldLabel htmlFor="connection-env-var">
                      Env var name
                    </FieldLabel>
                    <Input
                      id="connection-env-var"
                      value={envVar}
                      onChange={(e) => setEnvVar(e.target.value)}
                      placeholder="auto-generated if left blank"
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="connection-field">Field</FieldLabel>
                    <Select
                      value={field}
                      onValueChange={(value) => {
                        if (value) {
                          setField(value)
                        }
                      }}
                    >
                      <SelectTrigger id="connection-field">
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
                Leaving the env var blank generates a collision-safe name from
                the database and field, e.g.{' '}
                <code className="font-mono">MAIN_DATABASE_URL</code>.
              </FieldDescription>
            </Field>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
