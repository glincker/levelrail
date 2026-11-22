import { useState, type FormEvent } from 'react'
import { CloudGlassIcon } from '@thesvg/ui-react'
import type { Route53DnsSettings } from '../queries/route53Dns'
import {
  useDisconnectRoute53Dns,
  useUpdateRoute53DnsSettings,
} from '../queries/route53Dns'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { FieldDescription } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import {
  SettingsEnabledRow,
  SettingsFormActions,
  SettingsFormAlerts,
} from './SettingsCard'
import { DnsProviderFields } from './dns/DnsProviderFields'
import { DnsVerificationCallout } from './dns/DnsVerificationCallout'

// Instance-level Route53 DNS-01 credential: GET/PUT/DELETE
// /api/v1/settings/route53-dns. A second, independent ACME DNS-01
// provider from CloudflareDnsCard (routes/domains/index.tsx), not a
// replacement: if both are enabled, the ingress reconciler prefers
// Cloudflare. Carries its own local form state rather than reusing
// useTokenToggleForm (SettingsCard.tsx): that hook is shaped for a
// single token, and this provider needs an access key pair plus two
// optional non-secret fields (region, hosted zone id).
export function Route53DnsCard({
  settings,
}: Readonly<{ settings: Route53DnsSettings }>) {
  const updateSettings = useUpdateRoute53DnsSettings()
  const disconnect = useDisconnectRoute53Dns()

  const [enabled, setEnabled] = useState(settings.enabled)
  const [accessKeyId, setAccessKeyId] = useState('')
  const [secretAccessKey, setSecretAccessKey] = useState('')
  const [region, setRegion] = useState(settings.region ?? '')
  const [hostedZoneId, setHostedZoneId] = useState(
    settings.hosted_zone_id ?? '',
  )
  const [formError, setFormError] = useState<string | null>(null)

  const hasCredentials =
    settings.has_access_key_id && settings.has_secret_access_key
  const pending = updateSettings.isPending || disconnect.isPending

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)

    if ((accessKeyId.trim() === '') !== (secretAccessKey.trim() === '')) {
      setFormError('Access key ID and secret access key must be set together.')
      return
    }
    if (enabled && !hasCredentials && !accessKeyId.trim()) {
      setFormError(
        'An AWS access key ID and secret access key are required to enable DNS-01 for wildcard domains.',
      )
      return
    }

    updateSettings.mutate(
      {
        enabled,
        region: region.trim() || undefined,
        hosted_zone_id: hostedZoneId.trim() || undefined,
        access_key_id: accessKeyId.trim() || undefined,
        secret_access_key: secretAccessKey.trim() || undefined,
      },
      {
        onSuccess: () => {
          setAccessKeyId('')
          setSecretAccessKey('')
          toast.add({
            title: 'Route53 DNS-01 settings saved.',
            type: 'success',
          })
        },
      },
    )
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setAccessKeyId('')
        setSecretAccessKey('')
        toast.add({ title: 'Route53 DNS-01 disconnected.', type: 'success' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudGlassIcon className="size-5" />
          Wildcard domains (Route53 DNS-01)
        </CardTitle>
        <CardDescription>
          A second, independent DNS-01 provider from Cloudflare above: if both
          are enabled, Cloudflare takes precedence. Paste an AWS IAM access key
          pair scoped to <code>route53:ChangeResourceRecordSets</code>,{' '}
          <code>route53:ListResourceRecordSets</code>, and{' '}
          <code>route53:GetChange</code> on the hosted zone your wildcard
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
            ariaLabel="Route53 DNS-01 enabled"
          />

          <DnsProviderFields
            provider="route53"
            values={{
              access_key_id: accessKeyId,
              secret_access_key: secretAccessKey,
              region,
              hosted_zone_id: hostedZoneId,
            }}
            onChange={(key, value) => {
              if (key === 'access_key_id') setAccessKeyId(value)
              else if (key === 'secret_access_key') setSecretAccessKey(value)
              else if (key === 'region') setRegion(value)
              else if (key === 'hosted_zone_id') setHostedZoneId(value)
            }}
            idPrefix="route53-dns"
            hasSecret={hasCredentials}
            fieldExtra={(id) =>
              id === 'route53.secret_access_key' ? (
                <FieldDescription>
                  {hasCredentials
                    ? 'Credentials are already configured. Leave both fields blank to keep them.'
                    : 'No credentials set yet.'}
                </FieldDescription>
              ) : null
            }
          />

          <SettingsFormAlerts
            alerts={[
              formError ? { key: 'form', message: formError } : null,
              updateSettings.isError
                ? { key: 'update', message: updateSettings.error.message }
                : null,
            ]}
          />
          <DnsVerificationCallout
            verification={updateSettings.data?.verification}
          />

          <SettingsFormActions
            pending={pending}
            savePending={updateSettings.isPending}
            showSecondary={settings.enabled || hasCredentials}
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
