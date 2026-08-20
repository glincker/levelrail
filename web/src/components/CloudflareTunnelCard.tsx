import { useState } from 'react'
import {
  CloudCheckIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  MinusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import type { CloudflareTunnelSettings } from '../queries/cloudflareTunnel'
import {
  useDisconnectCloudflareTunnel,
  useUpdateCloudflareTunnelSettings,
} from '../queries/cloudflareTunnel'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'

const STATUS_VARIANT: Record<
  CloudflareTunnelSettings['status'],
  VariantProps<typeof badgeVariants>['variant']
> = {
  connected: 'success',
  error: 'destructive',
  disconnected: 'muted',
}

const STATUS_LABEL: Record<CloudflareTunnelSettings['status'], string> = {
  connected: 'Connected',
  error: 'Error',
  disconnected: 'Disconnected',
}

const STATUS_ICON: Record<CloudflareTunnelSettings['status'], Icon> = {
  connected: CheckCircleIcon,
  error: WarningCircleIcon,
  disconnected: MinusCircleIcon,
}

function StatusBadge({ status }: { status: CloudflareTunnelSettings['status'] }) {
  const StatusIcon = STATUS_ICON[status]
  return (
    <Badge variant={STATUS_VARIANT[status]} className="shrink-0">
      <StatusIcon className="size-3" />
      {STATUS_LABEL[status]}
    </Badge>
  )
}

// Instance-level Cloudflare Tunnel connection: GET/PUT/DELETE
// /api/v1/settings/cloudflare-tunnel. One tunnel per control plane (not
// per-app), matching store.CloudflareTunnelSettings' own single-row
// shape. The token is paste-once and write-only: this component never
// receives it back from the server, only HasToken (see
// queries/cloudflareTunnel.ts), the same pattern EmailSettingsCard
// already uses for smtp_password/ses_secret_access_key.
export function CloudflareTunnelCard({
  settings,
}: {
  settings: CloudflareTunnelSettings
}) {
  const [enabled, setEnabled] = useState(settings.enabled)
  const [token, setToken] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const updateSettings = useUpdateCloudflareTunnelSettings()
  const disconnect = useDisconnectCloudflareTunnel()

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (enabled && !settings.has_token && !token.trim()) {
      setFormError('A tunnel token is required to enable Cloudflare Tunnel.')
      return
    }
    updateSettings.mutate(
      { enabled, token: token.trim() || undefined },
      {
        onSuccess: () => {
          setToken('')
          toast.add({ title: 'Cloudflare Tunnel settings saved.', type: 'success' })
        },
      },
    )
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setToken('')
        toast.add({ title: 'Cloudflare Tunnel disconnected.', type: 'success' })
      },
    })
  }

  const pending = updateSettings.isPending || disconnect.isPending

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudCheckIcon className="size-4" />
          Cloudflare Tunnel
          <StatusBadge status={settings.status} />
        </CardTitle>
        <CardDescription>
          Expose this control plane through a Cloudflare Tunnel instead of
          opening an inbound port. Paste the tunnel token generated in your
          own Cloudflare Zero Trust dashboard; hostname routing stays
          configured on Cloudflare&apos;s side.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-foreground">Enabled</p>
              <p className="text-sm text-muted-foreground">
                Runs the cloudflared container connected to your tunnel.
              </p>
            </div>
            <Switch
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={pending}
              aria-label="Cloudflare Tunnel enabled"
            />
          </div>

          <Field>
            <FieldLabel htmlFor="cloudflare-tunnel-token">
              Tunnel token
            </FieldLabel>
            <Input
              id="cloudflare-tunnel-token"
              type="password"
              autoComplete="off"
              value={token}
              onChange={(e) => {
                setToken(e.target.value)
              }}
              disabled={pending}
              placeholder={settings.has_token ? '••••••••••••' : 'Paste your tunnel token'}
            />
            <FieldDescription>
              {settings.has_token
                ? 'A token is already configured. Leave blank to keep it.'
                : 'No token set yet.'}
            </FieldDescription>
          </Field>

          {settings.status === 'error' && settings.message ? (
            <Alert variant="destructive">
              <AlertDescription>{settings.message}</AlertDescription>
            </Alert>
          ) : null}
          {formError ? (
            <Alert variant="destructive">
              <AlertDescription>{formError}</AlertDescription>
            </Alert>
          ) : null}
          {updateSettings.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{updateSettings.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <div className="flex items-center gap-2">
            <Button type="submit" size="sm" disabled={pending}>
              {updateSettings.isPending ? 'Saving...' : 'Save'}
            </Button>
            {(settings.enabled || settings.has_token) && (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={handleDisconnect}
              >
                {disconnect.isPending ? 'Disconnecting...' : 'Disconnect'}
              </Button>
            )}
          </div>
        </form>
      </CardContent>
    </Card>
  )
}
