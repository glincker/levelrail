import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import {
  useClearDatabasePublicAccess,
  useSetDatabasePublicAccess,
} from '../queries/databases'
import type { DatabaseResource } from '../types/databaseDetail'

// Toggle + port surfacing for PUT/DELETE
// /api/v1/databases/{name}/public-access (internal/api/database_public_access.go):
// the missing link that lets an operator connect a database GUI tool
// (pgAdmin, TablePlus, RedisInsight) to a managed Postgres/Redis/MySQL
// from their own machine, not just from inside the Docker network.
// Rendered from routes/databases/$name/overview.tsx, the same section
// route BackupsSection already lives on.
//
// Off by default (internal/store/database.go's SetDatabasePublicAccess
// doc comment): the port field is only ever editable before enabling,
// since store.SetDatabasePublicAccess treats a re-enable with a
// different port as a real change, not an edit-in-place, and this UI
// keeps that one-directional shape rather than pretending the port can
// be changed without an explicit disable/re-enable round trip.
// Redis, unlike Postgres/MySQL, runs with no password for a managed
// database in this project (internal/reconcile/database/controller.go's
// own doc comment: "Redis can run safely with no password configured",
// true only as long as it's never reachable from outside the Docker
// network). Publishing its port to the host removes that "safely"
// clause entirely: exposure means unauthenticated read/write access to
// the whole dataset, not just "reachable," a materially different risk
// than Postgres/MySQL's real generated passwords. A generic warning
// here would flatten that difference, so Redis gets its own copy plus
// an explicit confirmation checkbox gating the toggle; the other two
// engines keep the single passive warning banner.
const NO_AUTH_ENGINES = new Set(['redis'])

export function DatabasePublicAccessCard({
  database,
}: {
  database: DatabaseResource
}) {
  const [portInput, setPortInput] = useState('')
  const [noAuthAcknowledged, setNoAuthAcknowledged] = useState(false)
  const setPublicAccess = useSetDatabasePublicAccess()
  const clearPublicAccess = useClearDatabasePublicAccess()
  const [copied, setCopied] = useState(false)

  const enabled = database.publicly_accessible ?? false
  const pending = setPublicAccess.isPending || clearPublicAccess.isPending
  const requiresNoAuthAck = NO_AUTH_ENGINES.has(database.engine)

  function handleToggle(next: boolean) {
    if (next) {
      if (requiresNoAuthAck && !noAuthAcknowledged) {
        return
      }
      const port = portInput.trim() ? Number(portInput) : undefined
      setPublicAccess.mutate(
        { name: database.name, port },
        {
          onSuccess: (result) => {
            toast.add({
              title: 'Public access enabled.',
              description: `Reachable on port ${result.public_port}.`,
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: 'Could not enable public access.',
              description: error.message,
              type: 'error',
            })
          },
        },
      )
      return
    }
    clearPublicAccess.mutate(database.name, {
      onSuccess: () => {
        setPortInput('')
        setCopied(false)
        setNoAuthAcknowledged(false)
      },
      onError: (error) => {
        toast.add({
          title: 'Could not disable public access.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const connectionString =
    enabled && database.public_port
      ? `${window.location.hostname}:${database.public_port}`
      : ''

  function copyConnectionString() {
    if (!connectionString) {
      return
    }
    void navigator.clipboard.writeText(connectionString).then(() => {
      setCopied(true)
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Public access</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">
              Expose to a host port
            </p>
            <p className="text-sm text-muted-foreground">
              Lets a database GUI tool (pgAdmin, TablePlus, RedisInsight)
              connect from your own machine.
            </p>
          </div>
          <Switch
            checked={enabled}
            onCheckedChange={handleToggle}
            disabled={pending || (requiresNoAuthAck && !noAuthAcknowledged)}
            aria-label="Publicly accessible"
          />
        </div>

        {!enabled && requiresNoAuthAck && (
          <div className="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
            <div className="flex items-start gap-2">
              <WarningIcon className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                This database has no password configured. Enabling public
                access lets anyone who can reach this host read and write
                the entire dataset with no authentication.
              </p>
            </div>
            <label className="flex items-center gap-2 pl-6 text-sm">
              <Checkbox
                checked={noAuthAcknowledged}
                onCheckedChange={(checked) => {
                  setNoAuthAcknowledged(checked === true)
                }}
              />
              I understand this database has no password.
            </label>
          </div>
        )}

        {!enabled && (
          <Field>
            <FieldLabel htmlFor="public-access-port">
              Host port (optional)
            </FieldLabel>
            <Input
              id="public-access-port"
              type="number"
              inputMode="numeric"
              placeholder="Auto-assign"
              value={portInput}
              onChange={(e) => {
                setPortInput(e.target.value)
              }}
              disabled={pending}
              className="max-w-40"
            />
            <FieldDescription>
              Leave blank to auto-assign a free port.
            </FieldDescription>
          </Field>
        )}

        {enabled && connectionString ? (
          <div className="space-y-2">
            <div className="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
              <WarningIcon className="mt-0.5 size-4 shrink-0" />
              <p className="text-sm">
                {requiresNoAuthAck
                  ? 'This database has no password. This port is reachable from any network that can reach this host, granting full unauthenticated access to the dataset.'
                  : 'This port is reachable from any network that can reach this host, not just your own machine. Make sure your firewall or network trusts that before relying on this in production.'}
              </p>
            </div>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {connectionString}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copyConnectionString}
              >
                {copied ? <CheckIcon /> : <CopyIcon />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
