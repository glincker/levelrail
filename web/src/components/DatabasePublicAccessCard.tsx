import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { Banner } from '@/components/ui/banner'
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
// Redis, KeyDB, and Dragonfly all run passwordless by default in this
// project (internal/reconcile/database/controller.go and address.go's
// SupportsField group all three as the same no-auth family: KeyDB and
// Dragonfly are Redis-protocol-compatible forks that reuse Redis's
// posture directly). Publishing any of their ports to the host means
// unauthenticated read/write access to the whole dataset, a materially
// different risk than Postgres/MySQL's generated passwords, so this
// family gets its own copy plus an explicit confirmation checkbox
// gating the toggle; the other engines keep the single passive warning.
const NO_AUTH_ENGINES = new Set(['redis', 'keydb', 'dragonfly', 'valkey'])

// BIND_ADDRESS_MODES mirrors PortEditor's own identically-named
// constant: internal/bindaddr's Private/Public shorthand plus a
// UI-only "custom" mode for a literal IP.
const BIND_ADDRESS_MODES = ['private', 'public', 'custom'] as const
type BindAddressMode = (typeof BIND_ADDRESS_MODES)[number]

const BIND_ADDRESS_LABELS: Record<BindAddressMode, string> = {
  private: 'Private (127.0.0.1)',
  public: 'Public (0.0.0.0)',
  custom: 'Custom IP',
}

export function DatabasePublicAccessCard({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const [portInput, setPortInput] = useState('')
  const [bindAddressMode, setBindAddressMode] =
    useState<BindAddressMode>('private')
  const [customBindAddress, setCustomBindAddress] = useState('')
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
      if (bindAddressMode === 'custom' && !customBindAddress.trim()) {
        return
      }
      const port = portInput.trim() ? Number(portInput) : undefined
      const bindAddress =
        bindAddressMode === 'custom'
          ? customBindAddress.trim()
          : bindAddressMode
      setPublicAccess.mutate(
        { name: database.name, port, bindAddress },
        {
          onSuccess: (result) => {
            toast.add({
              title: 'Public access enabled.',
              description: `Reachable on port ${result.public_port}, bound to ${result.bind_address}.`,
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
        setBindAddressMode('private')
        setCustomBindAddress('')
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
          <>
            <Banner
              variant="warning"
              title={t('publicAccess.noPasswordWarning')}
            />
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={noAuthAcknowledged}
                onCheckedChange={(checked) => {
                  setNoAuthAcknowledged(checked === true)
                }}
              />
              I understand this database has no password.
            </label>
          </>
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

        {!enabled && (
          <Field>
            <FieldLabel htmlFor="public-access-bind-address">
              Network interface
            </FieldLabel>
            <Select
              value={bindAddressMode}
              onValueChange={(value) => {
                setBindAddressMode(value as BindAddressMode)
              }}
            >
              <SelectTrigger id="public-access-bind-address" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {BIND_ADDRESS_MODES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {BIND_ADDRESS_LABELS[value]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              Private is reachable only from this host. Public exposes it to any
              network that can reach this host.
            </FieldDescription>
            {bindAddressMode === 'custom' && (
              <Input
                id="public-access-custom-bind-address"
                placeholder="10.0.0.5"
                value={customBindAddress}
                onChange={(e) => {
                  setCustomBindAddress(e.target.value)
                }}
                disabled={pending}
                className="mt-2 max-w-48"
              />
            )}
          </Field>
        )}

        {enabled && connectionString ? (
          <div className="space-y-4">
            {database.public_bind_address ? (
              <p className="text-sm text-muted-foreground">
                Bound to{' '}
                {BIND_ADDRESS_LABELS[
                  database.public_bind_address as BindAddressMode
                ] ?? database.public_bind_address}
              </p>
            ) : null}
            <Banner
              variant="warning"
              title={
                requiresNoAuthAck
                  ? t('publicAccess.noPasswordOpen')
                  : t('publicAccess.openPort')
              }
            />
            <div className="flex items-center gap-2 rounded-md bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {connectionString}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copyConnectionString}
              >
                {copied ? (
                  <CheckGlassIcon size="1.4em" />
                ) : (
                  <CopyGlassIcon size="1.4em" />
                )}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
