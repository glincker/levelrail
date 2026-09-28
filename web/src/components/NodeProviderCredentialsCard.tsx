import { useState } from 'react'
import { CloudIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { StatusPill } from './kit/StatusPill'
import {
  useNodeProviders,
  useSetNodeProviderCredential,
} from '../queries/nodeProvision'
import type { NodeProviderResource } from '../types/nodeProvision'

const PROVIDER_LABELS: Record<string, string> = {
  hetzner: 'Hetzner',
  digitalocean: 'DigitalOcean',
  aws: 'AWS',
  azure: 'Azure',
  gcp: 'Google Cloud',
}

// Hetzner and DigitalOcean take a plain API token; Azure and GCP take a
// JSON credential pasted into the same single-line field (see
// docs/node-provisioning.md), so the field's own label and helper text
// differ by provider instead of always reading "API token". AWS's fields
// are rendered separately below since it splits into several inputs.
const PROVIDER_FIELD_LABEL: Record<string, string> = {
  azure: 'Service principal (JSON)',
  gcp: 'Service account key (JSON)',
}

const PROVIDER_PLACEHOLDER: Record<string, string> = {
  azure:
    '{"tenant_id":"...","client_id":"...","client_secret":"...","subscription_id":"...","resource_group":"..."}',
  gcp: 'Paste the service account key JSON, minified to one line',
}

// Instance-level cloud provider credentials for "nodes provision" and the
// nodes page's own Add node wizard: GET/POST /api/v1/node-providers. One
// mini-form per known provider, the same "list of independent cards"
// shape RegistryCredentialTable establishes for a different multi-
// credential resource, simplified here since there are only ever two
// rows and neither can be deleted (only replaced).
export function NodeProviderCredentialsCard() {
  const providers = useNodeProviders()
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudIcon className="size-4" />
          Cloud node provisioning
        </CardTitle>
        <CardDescription>
          API tokens for creating servers automatically from the Nodes page.
          Used only to create and inspect VMs; day to day operation never
          touches SSH.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {providers.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : providers.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{providers.error.message}</AlertDescription>
          </Alert>
        ) : (
          (providers.data ?? []).map((p) => (
            <ProviderForm key={p.provider} provider={p} />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function ProviderForm({ provider }: { provider: NodeProviderResource }) {
  const isAWS = provider.provider === 'aws'
  const [token, setToken] = useState('')
  const [secretAccessKey, setSecretAccessKey] = useState('')
  const [sessionToken, setSessionToken] = useState('')
  const [region, setRegion] = useState('')
  const [roleARN, setRoleARN] = useState('')
  const [useAmbient, setUseAmbient] = useState(false)
  const setCredential = useSetNodeProviderCredential()
  const label = PROVIDER_LABELS[provider.provider] ?? provider.provider
  const fieldLabel = PROVIDER_FIELD_LABEL[provider.provider] ?? 'API token'

  const canSubmit = isAWS
    ? useAmbient || (token !== '' && secretAccessKey !== '')
    : token !== ''

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    setCredential.mutate(
      {
        provider: provider.provider,
        token,
        ...(isAWS
          ? {
              secret_access_key: secretAccessKey,
              session_token: sessionToken || undefined,
              region: region || undefined,
              role_arn: roleARN || undefined,
              use_ambient_credentials: useAmbient,
            }
          : {}),
      },
      {
        onSuccess: () => {
          setToken('')
          setSecretAccessKey('')
          setSessionToken('')
        },
      },
    )
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="space-y-3 rounded-lg border border-border p-3"
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <StatusPill
          tone={provider.has_token ? 'success' : 'neutral'}
          label={provider.has_token ? 'connected' : 'not connected'}
          size="sm"
        />
      </div>
      {!isAWS ? (
        <Field>
          <FieldLabel htmlFor={`node-provider-token-${provider.provider}`}>
            {fieldLabel}
          </FieldLabel>
          <Input
            id={`node-provider-token-${provider.provider}`}
            type="password"
            autoComplete="off"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={
              provider.has_token
                ? '••••••••••••'
                : (PROVIDER_PLACEHOLDER[provider.provider] ??
                  `Paste your ${label} API token`)
            }
          />
          <FieldDescription>
            {provider.has_token
              ? 'A token is already stored. Paste a new one to replace it.'
              : 'Never echoed back once saved.'}
          </FieldDescription>
        </Field>
      ) : null}
      {isAWS ? (
        <div className="flex items-center gap-2">
          <Checkbox
            id={`node-provider-aws-ambient-${provider.provider}`}
            checked={useAmbient}
            onCheckedChange={(v) => setUseAmbient(v === true)}
          />
          <FieldLabel
            htmlFor={`node-provider-aws-ambient-${provider.provider}`}
            className="font-normal"
          >
            Use this control plane&apos;s own AWS identity (env vars, shared
            config, or an EC2 instance profile) instead of a stored key
          </FieldLabel>
        </div>
      ) : null}
      {!isAWS || !useAmbient ? (
        <Field>
          <FieldLabel htmlFor={`node-provider-token-${provider.provider}`}>
            {isAWS ? 'Access key ID' : 'API token'}
          </FieldLabel>
          <Input
            id={`node-provider-token-${provider.provider}`}
            type="password"
            autoComplete="off"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder={
              provider.has_token
                ? '••••••••••••'
                : isAWS
                  ? 'AKIA...'
                  : `Paste your ${label} API token`
            }
          />
          <FieldDescription>
            {provider.has_token
              ? 'A credential is already stored. Fill in a new one to replace it.'
              : 'Never echoed back once saved.'}
          </FieldDescription>
        </Field>
      ) : null}
      {isAWS && !useAmbient ? (
        <Field>
          <FieldLabel htmlFor={`node-provider-aws-secret-${provider.provider}`}>
            Secret access key
          </FieldLabel>
          <Input
            id={`node-provider-aws-secret-${provider.provider}`}
            type="password"
            autoComplete="off"
            value={secretAccessKey}
            onChange={(e) => setSecretAccessKey(e.target.value)}
          />
        </Field>
      ) : null}
      {isAWS ? (
        <>
          <Field>
            <FieldLabel
              htmlFor={`node-provider-aws-session-token-${provider.provider}`}
            >
              Session token (optional)
            </FieldLabel>
            <Input
              id={`node-provider-aws-session-token-${provider.provider}`}
              type="password"
              autoComplete="off"
              value={sessionToken}
              onChange={(e) => setSessionToken(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel
              htmlFor={`node-provider-aws-region-${provider.provider}`}
            >
              Region (optional)
            </FieldLabel>
            <Input
              id={`node-provider-aws-region-${provider.provider}`}
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              placeholder="us-east-1"
              className="font-mono text-xs"
            />
          </Field>
          <Field>
            <FieldLabel
              htmlFor={`node-provider-aws-role-arn-${provider.provider}`}
            >
              Role ARN (optional)
            </FieldLabel>
            <Input
              id={`node-provider-aws-role-arn-${provider.provider}`}
              value={roleARN}
              onChange={(e) => setRoleARN(e.target.value)}
              placeholder="arn:aws:iam::123456789012:role/example-provisioner"
              className="font-mono text-xs"
            />
            <FieldDescription>
              When set, assumed via STS before use: the credential above then
              only needs sts:AssumeRole on this role, not direct EC2
              permissions.
            </FieldDescription>
          </Field>
        </>
      ) : null}
      {setCredential.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{setCredential.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="submit"
        size="sm"
        disabled={!canSubmit || setCredential.isPending}
      >
        {setCredential.isPending ? 'Saving...' : 'Save'}
      </Button>
    </form>
  )
}
