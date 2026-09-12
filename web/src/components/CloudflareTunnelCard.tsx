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
import type { badgeVariants } from '@/components/ui/badge'
import { StatusBadge } from '@/components/ui/status-badge'
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
  const updateSettings = useUpdateCloudflareTunnelSettings()
  const disconnect = useDisconnectCloudflareTunnel()
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
    requiredTokenMessage: 'A tunnel token is required to enable Cloudflare Tunnel.',
    saveSuccessTitle: 'Cloudflare Tunnel settings saved.',
    disconnectSuccessTitle: 'Cloudflare Tunnel disconnected.',
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudCheckIcon className="size-4" />
          Cloudflare Tunnel
          <StatusBadge
            variant={STATUS_VARIANT[settings.status]}
            label={STATUS_LABEL[settings.status]}
            icon={STATUS_ICON[settings.status]}
          />
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
          <SettingsEnabledRow
            description="Runs the cloudflared container connected to your tunnel."
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={pending}
            ariaLabel="Cloudflare Tunnel enabled"
          />

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

          <SettingsFormAlerts
            alerts={[
              settings.status === 'error' && settings.message
                ? { key: 'status', message: settings.message }
                : null,
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
