import { useState } from 'react'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { CloudflareDnsSettings } from '../queries/cloudflareDns'
import {
  useDisconnectCloudflareDns,
  useUpdateCloudflareDnsSettings,
} from '../queries/cloudflareDns'
import { Alert, AlertDescription } from '@/components/ui/alert'
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

// Instance-level Cloudflare DNS-01 credential: GET/PUT/DELETE
// /api/v1/settings/cloudflare-dns. Lives next to IngressSettingsCard
// (routes/domains/index.tsx) because it only matters once ACME is
// enabled there: a wildcard domain (e.g. "*.example.com", a leading
// "*." is the whole convention, no separate toggle marks it) needs
// DNS-01 to prove control, which HTTP-01 structurally cannot do. A
// distinct credential from Cloudflare Tunnel's own connector token
// (settings/cloudflare-tunnel.tsx): this one is a scoped Cloudflare API
// token with Zone:DNS:Edit permission, not a tunnel connector token.
export function CloudflareDnsCard({
  settings,
}: {
  settings: CloudflareDnsSettings
}) {
  const [enabled, setEnabled] = useState(settings.enabled)
  const [token, setToken] = useState('')
  const [formError, setFormError] = useState<string | null>(null)
  const updateSettings = useUpdateCloudflareDnsSettings()
  const disconnect = useDisconnectCloudflareDns()

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (enabled && !settings.has_token && !token.trim()) {
      setFormError(
        'A Cloudflare API token is required to enable DNS-01 for wildcard domains.',
      )
      return
    }
    updateSettings.mutate(
      { enabled, token: token.trim() || undefined },
      {
        onSuccess: () => {
          setToken('')
          toast.add({ title: 'Cloudflare DNS-01 settings saved.', type: 'success' })
        },
      },
    )
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setToken('')
        toast.add({ title: 'Cloudflare DNS-01 disconnected.', type: 'success' })
      },
    })
  }

  const pending = updateSettings.isPending || disconnect.isPending

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckIcon className="size-4" />
          Wildcard domains (Cloudflare DNS-01)
        </CardTitle>
        <CardDescription>
          A wildcard domain like <code>*.example.com</code> needs the ACME
          DNS-01 challenge to get a real certificate; the default HTTP-01
          challenge cannot prove control of one. Paste a Cloudflare API
          token scoped to Zone:DNS:Edit for the zone your wildcard
          domains live under. Requires ACME to also be enabled above.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-5">
          <div className="flex items-center justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-foreground">Enabled</p>
              <p className="text-sm text-muted-foreground">
                Issues wildcard certificates via DNS-01 instead of skipping
                them.
              </p>
            </div>
            <Switch
              checked={enabled}
              onCheckedChange={setEnabled}
              disabled={pending}
              aria-label="Cloudflare DNS-01 enabled"
            />
          </div>

          <Field>
            <FieldLabel htmlFor="cloudflare-dns-token">
              Cloudflare API token
            </FieldLabel>
            <Input
              id="cloudflare-dns-token"
              type="password"
              autoComplete="off"
              value={token}
              onChange={(e) => {
                setToken(e.target.value)
              }}
              disabled={pending}
              placeholder={
                settings.has_token ? '••••••••••••' : 'Paste your Cloudflare API token'
              }
            />
            <FieldDescription>
              {settings.has_token
                ? 'A token is already configured. Leave blank to keep it.'
                : 'No token set yet. Needs Zone:DNS:Edit permission, not the global API key.'}
            </FieldDescription>
          </Field>

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
