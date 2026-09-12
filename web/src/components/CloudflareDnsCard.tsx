import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { CloudflareDnsSettings } from '../queries/cloudflareDns'
import {
  useDisconnectCloudflareDns,
  useUpdateCloudflareDnsSettings,
} from '../queries/cloudflareDns'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  SettingsEnabledRow,
  SettingsFormActions,
  SettingsFormAlerts,
  useTokenToggleForm,
} from './SettingsCard'

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
  const updateSettings = useUpdateCloudflareDnsSettings()
  const disconnect = useDisconnectCloudflareDns()
  const {
    enabled,
    setEnabled,
    token,
    setToken,
    formError,
    handleSubmit,
    handleDisconnect,
    pending,
  } = useTokenToggleForm({
    settings,
    updateMutation: updateSettings,
    disconnectMutation: disconnect,
    requiredTokenMessage:
      'A Cloudflare API token is required to enable DNS-01 for wildcard domains.',
    saveSuccessTitle: 'Cloudflare DNS-01 settings saved.',
    disconnectSuccessTitle: 'Cloudflare DNS-01 disconnected.',
  })

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
          <SettingsEnabledRow
            description="Issues wildcard certificates via DNS-01 instead of skipping them."
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={pending}
            ariaLabel="Cloudflare DNS-01 enabled"
          />

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

          <SettingsFormAlerts
            alerts={[
              formError ? { key: 'form', message: formError } : null,
              updateSettings.isError
                ? { key: 'update', message: updateSettings.error.message }
                : null,
            ]}
          />

          <SettingsFormActions
            pending={pending}
            savePending={updateSettings.isPending}
            showSecondary={settings.enabled || settings.has_token}
            secondaryPending={disconnect.isPending}
            secondaryLabel="Disconnect"
            secondaryPendingLabel="Disconnecting..."
            onSecondaryClick={handleDisconnect}
          />
        </form>
      </CardContent>
    </Card>
  )
}
